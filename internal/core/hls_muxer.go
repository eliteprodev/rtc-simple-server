package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/ringbuffer"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/aler9/gortsplib/pkg/rtph264"
	"github.com/pion/rtp"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/hls"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

const (
	closeCheckPeriod     = 1 * time.Second
	closeAfterInactivity = 60 * time.Second
)

const index = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
html, body {
	margin: 0;
	padding: 0;
	height: 100%;
}
#video {
	width: 100%;
	height: 100%;
	background: black;
}
</style>
</head>
<body>

<video id="video" muted controls autoplay></video>

<script src="https://cdn.jsdelivr.net/npm/hls.js@1.1.1"></script>

<script>

const create = () => {
	const video = document.getElementById('video');

	if (video.canPlayType('application/vnd.apple.mpegurl')) {
		// since it's not possible to detect timeout errors in iOS,
		// wait for the playlist to be available before starting the stream
		fetch('stream.m3u8')
			.then(() => {
				video.src = 'index.m3u8';
				video.play();
			});

	} else {
		const hls = new Hls({
			progressive: true,
		});

		hls.on(Hls.Events.ERROR, (evt, data) => {
			if (data.fatal) {
				hls.destroy();

				setTimeout(create, 2000);
			}
		});

		hls.loadSource('index.m3u8');
		hls.attachMedia(video);

		video.play();
	}
};

window.addEventListener('DOMContentLoaded', create);

</script>

</body>
</html>
`

type hlsMuxerResponse struct {
	Status int
	Header map[string]string
	Body   io.Reader
}

type hlsMuxerRequest struct {
	Dir  string
	File string
	Req  *http.Request
	Res  chan hlsMuxerResponse
}

type hlsMuxerTrackIDPayloadPair struct {
	trackID int
	buf     []byte
}

type hlsMuxerPathManager interface {
	onReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes
}

type hlsMuxerParent interface {
	log(logger.Level, string, ...interface{})
	onMuxerClose(*hlsMuxer)
}

type hlsMuxer struct {
	name               string
	hlsAlwaysRemux     bool
	hlsSegmentCount    int
	hlsSegmentDuration conf.StringDuration
	readBufferCount    int
	wg                 *sync.WaitGroup
	pathName           string
	pathManager        hlsMuxerPathManager
	parent             hlsMuxerParent

	ctx             context.Context
	ctxCancel       func()
	path            *path
	ringBuffer      *ringbuffer.RingBuffer
	lastRequestTime *int64
	muxer           *hls.Muxer
	requests        []hlsMuxerRequest

	// in
	request                chan hlsMuxerRequest
	hlsServerAPIMuxersList chan hlsServerAPIMuxersListSubReq
}

func newHLSMuxer(
	parentCtx context.Context,
	name string,
	hlsAlwaysRemux bool,
	hlsSegmentCount int,
	hlsSegmentDuration conf.StringDuration,
	readBufferCount int,
	wg *sync.WaitGroup,
	pathName string,
	pathManager hlsMuxerPathManager,
	parent hlsMuxerParent) *hlsMuxer {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	m := &hlsMuxer{
		name:               name,
		hlsAlwaysRemux:     hlsAlwaysRemux,
		hlsSegmentCount:    hlsSegmentCount,
		hlsSegmentDuration: hlsSegmentDuration,
		readBufferCount:    readBufferCount,
		wg:                 wg,
		pathName:           pathName,
		pathManager:        pathManager,
		parent:             parent,
		ctx:                ctx,
		ctxCancel:          ctxCancel,
		lastRequestTime: func() *int64 {
			v := time.Now().Unix()
			return &v
		}(),
		request:                make(chan hlsMuxerRequest),
		hlsServerAPIMuxersList: make(chan hlsServerAPIMuxersListSubReq),
	}

	m.log(logger.Info, "opened")

	m.wg.Add(1)
	go m.run()

	return m
}

func (m *hlsMuxer) close() {
	m.ctxCancel()
}

func (m *hlsMuxer) log(level logger.Level, format string, args ...interface{}) {
	m.parent.log(level, "[muxer %s] "+format, append([]interface{}{m.pathName}, args...)...)
}

// PathName returns the path name.
func (m *hlsMuxer) PathName() string {
	return m.pathName
}

func (m *hlsMuxer) run() {
	defer m.wg.Done()

	innerCtx, innerCtxCancel := context.WithCancel(context.Background())
	innerReady := make(chan struct{})
	innerErr := make(chan error)
	go func() {
		innerErr <- m.runInner(innerCtx, innerReady)
	}()

	isReady := false

	err := func() error {
		for {
			select {
			case <-m.ctx.Done():
				innerCtxCancel()
				<-innerErr
				return errors.New("terminated")

			case req := <-m.request:
				if isReady {
					req.Res <- m.handleRequest(req)
				} else {
					m.requests = append(m.requests, req)
				}

			case req := <-m.hlsServerAPIMuxersList:
				req.Data.Items[m.name] = hlsServerAPIMuxersListItem{
					LastRequest: time.Unix(atomic.LoadInt64(m.lastRequestTime), 0).String(),
				}
				close(req.Res)

			case <-innerReady:
				isReady = true
				for _, req := range m.requests {
					req.Res <- m.handleRequest(req)
				}
				m.requests = nil

			case err := <-innerErr:
				innerCtxCancel()
				return err
			}
		}
	}()

	m.ctxCancel()

	for _, req := range m.requests {
		req.Res <- hlsMuxerResponse{Status: http.StatusNotFound}
	}

	m.parent.onMuxerClose(m)

	m.log(logger.Info, "closed (%v)", err)
}

func (m *hlsMuxer) runInner(innerCtx context.Context, innerReady chan struct{}) error {
	res := m.pathManager.onReaderSetupPlay(pathReaderSetupPlayReq{
		Author:              m,
		PathName:            m.pathName,
		IP:                  nil,
		ValidateCredentials: nil,
	})
	if res.Err != nil {
		return res.Err
	}

	m.path = res.Path

	defer func() {
		m.path.onReaderRemove(pathReaderRemoveReq{Author: m})
	}()

	var videoTrack *gortsplib.Track
	videoTrackID := -1
	var h264Decoder *rtph264.Decoder
	var audioTrack *gortsplib.Track
	audioTrackID := -1
	var aacDecoder *rtpaac.Decoder

	for i, t := range res.Stream.tracks() {
		if t.IsH264() {
			if videoTrack != nil {
				return fmt.Errorf("can't read track %d with HLS: too many tracks", i+1)
			}

			videoTrack = t
			videoTrackID = i

			h264Decoder = rtph264.NewDecoder()
		} else if t.IsAAC() {
			if audioTrack != nil {
				return fmt.Errorf("can't read track %d with HLS: too many tracks", i+1)
			}

			audioTrack = t
			audioTrackID = i

			conf, err := t.ExtractConfigAAC()
			if err != nil {
				return err
			}

			aacDecoder = rtpaac.NewDecoder(conf.SampleRate)
		}
	}

	if videoTrack == nil && audioTrack == nil {
		return fmt.Errorf("the stream doesn't contain an H264 track or an AAC track")
	}

	var err error
	m.muxer, err = hls.NewMuxer(
		m.hlsSegmentCount,
		time.Duration(m.hlsSegmentDuration),
		videoTrack,
		audioTrack,
	)
	if err != nil {
		return err
	}
	defer m.muxer.Close()

	innerReady <- struct{}{}

	m.ringBuffer = ringbuffer.New(uint64(m.readBufferCount))

	m.path.onReaderPlay(pathReaderPlayReq{Author: m})

	writerDone := make(chan error)
	go func() {
		writerDone <- func() error {
			for {
				data, ok := m.ringBuffer.Pull()
				if !ok {
					return fmt.Errorf("terminated")
				}
				pair := data.(hlsMuxerTrackIDPayloadPair)

				if videoTrack != nil && pair.trackID == videoTrackID {
					var pkt rtp.Packet
					err := pkt.Unmarshal(pair.buf)
					if err != nil {
						m.log(logger.Warn, "unable to decode RTP packet: %v", err)
						continue
					}

					nalus, pts, err := h264Decoder.DecodeUntilMarker(&pkt)
					if err != nil {
						if err != rtph264.ErrMorePacketsNeeded &&
							err != rtph264.ErrNonStartingPacketAndNoPrevious {
							m.log(logger.Warn, "unable to decode video track: %v", err)
						}
						continue
					}

					err = m.muxer.WriteH264(pts, nalus)
					if err != nil {
						return err
					}
				} else if audioTrack != nil && pair.trackID == audioTrackID {
					var pkt rtp.Packet
					err := pkt.Unmarshal(pair.buf)
					if err != nil {
						m.log(logger.Warn, "unable to decode RTP packet: %v", err)
						continue
					}

					aus, pts, err := aacDecoder.Decode(&pkt)
					if err != nil {
						if err != rtpaac.ErrMorePacketsNeeded {
							m.log(logger.Warn, "unable to decode audio track: %v", err)
						}
						continue
					}

					err = m.muxer.WriteAAC(pts, aus)
					if err != nil {
						return err
					}
				}
			}
		}()
	}()

	closeCheckTicker := time.NewTicker(closeCheckPeriod)
	defer closeCheckTicker.Stop()

	for {
		select {
		case <-closeCheckTicker.C:
			t := time.Unix(atomic.LoadInt64(m.lastRequestTime), 0)
			if !m.hlsAlwaysRemux && time.Since(t) >= closeAfterInactivity {
				m.ringBuffer.Close()
				<-writerDone
				return nil
			}

		case err := <-writerDone:
			return err

		case <-innerCtx.Done():
			m.ringBuffer.Close()
			<-writerDone
			return nil
		}
	}
}

func (m *hlsMuxer) handleRequest(req hlsMuxerRequest) hlsMuxerResponse {
	atomic.StoreInt64(m.lastRequestTime, time.Now().Unix())

	conf := m.path.Conf()

	if conf.ReadIPs != nil {
		tmp, _, _ := net.SplitHostPort(req.Req.RemoteAddr)
		ip := net.ParseIP(tmp)
		if !ipEqualOrInRange(ip, conf.ReadIPs) {
			m.log(logger.Info, "ip '%s' not allowed", ip)
			return hlsMuxerResponse{Status: http.StatusUnauthorized}
		}
	}

	if conf.ReadUser != "" {
		user, pass, ok := req.Req.BasicAuth()
		if !ok || user != string(conf.ReadUser) || pass != string(conf.ReadPass) {
			return hlsMuxerResponse{
				Status: http.StatusUnauthorized,
				Header: map[string]string{
					"WWW-Authenticate": `Basic realm="rtsp-simple-server"`,
				},
			}
		}
	}

	switch {
	case req.File == "index.m3u8":
		return hlsMuxerResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": `application/x-mpegURL`,
			},
			Body: m.muxer.PrimaryPlaylist(),
		}

	case req.File == "stream.m3u8":
		return hlsMuxerResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": `application/x-mpegURL`,
			},
			Body: m.muxer.StreamPlaylist(),
		}

	case strings.HasSuffix(req.File, ".ts"):
		r := m.muxer.Segment(req.File)
		if r == nil {
			return hlsMuxerResponse{Status: http.StatusNotFound}
		}

		return hlsMuxerResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": `video/MP2T`,
			},
			Body: r,
		}

	case req.File == "":
		return hlsMuxerResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": `text/html`,
			},
			Body: bytes.NewReader([]byte(index)),
		}

	default:
		return hlsMuxerResponse{Status: http.StatusNotFound}
	}
}

// onRequest is called by hlsserver.Server (forwarded from ServeHTTP).
func (m *hlsMuxer) onRequest(req hlsMuxerRequest) {
	select {
	case m.request <- req:
	case <-m.ctx.Done():
		req.Res <- hlsMuxerResponse{Status: http.StatusNotFound}
	}
}

// onReaderAccepted implements reader.
func (m *hlsMuxer) onReaderAccepted() {
	m.log(logger.Info, "is converting into HLS")
}

// onReaderPacketRTP implements reader.
func (m *hlsMuxer) onReaderPacketRTP(trackID int, payload []byte) {
	m.ringBuffer.Push(hlsMuxerTrackIDPayloadPair{trackID, payload})
}

// onReaderPacketRTCP implements reader.
func (m *hlsMuxer) onReaderPacketRTCP(trackID int, payload []byte) {
}

// onReaderAPIDescribe implements reader.
func (m *hlsMuxer) onReaderAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"hlsMuxer"}
}

// onAPIHLSMuxersList is called by api.
func (m *hlsMuxer) onAPIHLSMuxersList(req hlsServerAPIMuxersListSubReq) {
	req.Res = make(chan struct{})
	select {
	case m.hlsServerAPIMuxersList <- req:
		<-req.Res

	case <-m.ctx.Done():
	}
}

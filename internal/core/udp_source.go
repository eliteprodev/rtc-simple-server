package core

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/asticode/go-astits"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	"golang.org/x/net/ipv4"

	"github.com/aler9/mediamtx/internal/conf"
	"github.com/aler9/mediamtx/internal/formatprocessor"
	"github.com/aler9/mediamtx/internal/logger"
)

const (
	multicastTTL = 16
	udpMTU       = 1472
)

var opusDurations = [32]int{
	480, 960, 1920, 2880, /* Silk NB */
	480, 960, 1920, 2880, /* Silk MB */
	480, 960, 1920, 2880, /* Silk WB */
	480, 960, /* Hybrid SWB */
	480, 960, /* Hybrid FB */
	120, 240, 480, 960, /* CELT NB */
	120, 240, 480, 960, /* CELT NB */
	120, 240, 480, 960, /* CELT NB */
	120, 240, 480, 960, /* CELT NB */
}

func opusGetPacketDuration(pkt []byte) time.Duration {
	if len(pkt) == 0 {
		return 0
	}

	frameDuration := opusDurations[pkt[0]>>3]

	frameCount := 0
	switch pkt[0] & 3 {
	case 0:
		frameCount = 1
	case 1:
		frameCount = 2
	case 2:
		frameCount = 2
	case 3:
		if len(pkt) < 2 {
			return 0
		}
		frameCount = int(pkt[1] & 63)
	}

	return (time.Duration(frameDuration) * time.Duration(frameCount) * time.Millisecond) / 48
}

type readerFunc func([]byte) (int, error)

func (rf readerFunc) Read(p []byte) (int, error) {
	return rf(p)
}

type udpSourceParent interface {
	log(logger.Level, string, ...interface{})
	sourceStaticImplSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	sourceStaticImplSetNotReady(req pathSourceStaticSetNotReadyReq)
}

type udpSource struct {
	readTimeout conf.StringDuration
	parent      udpSourceParent
}

func newUDPSource(
	readTimeout conf.StringDuration,
	parent udpSourceParent,
) *udpSource {
	return &udpSource{
		readTimeout: readTimeout,
		parent:      parent,
	}
}

func (s *udpSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[udp source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *udpSource) run(ctx context.Context, cnf *conf.PathConf, reloadConf chan *conf.PathConf) error {
	s.Log(logger.Debug, "connecting")

	hostPort := cnf.Source[len("udp://"):]

	pc, err := net.ListenPacket("udp", hostPort)
	if err != nil {
		return err
	}
	defer pc.Close()

	host, _, _ := net.SplitHostPort(hostPort)
	ip := net.ParseIP(host)

	if ip.IsMulticast() {
		p := ipv4.NewPacketConn(pc)

		err = p.SetMulticastTTL(multicastTTL)
		if err != nil {
			return err
		}

		intfs, err := net.Interfaces()
		if err != nil {
			return err
		}

		for _, intf := range intfs {
			err := p.JoinGroup(&intf, &net.UDPAddr{IP: ip})
			if err != nil {
				return err
			}
		}
	}

	midbuffer := make([]byte, 0, udpMTU)
	midbufferPos := 0

	readPacket := func(buf []byte) (int, error) {
		if midbufferPos < len(midbuffer) {
			n := copy(buf, midbuffer[midbufferPos:])
			midbufferPos += n
			return n, nil
		}

		mn, _, err := pc.ReadFrom(midbuffer[:cap(midbuffer)])
		if err != nil {
			return 0, err
		}

		if (mn % 188) != 0 {
			return 0, fmt.Errorf("received packet with size %d not multiple of 188", mn)
		}

		midbuffer = midbuffer[:mn]
		n := copy(buf, midbuffer)
		midbufferPos = n
		return n, nil
	}

	dem := astits.NewDemuxer(
		context.Background(),
		readerFunc(readPacket),
		astits.DemuxerOptPacketSize(188))

	readerErr := make(chan error)

	go func() {
		readerErr <- func() error {
			pc.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
			tracks, err := mpegts.FindTracks(dem)
			if err != nil {
				return err
			}

			var medias media.Medias
			mediaCallbacks := make(map[uint16]func(time.Duration, []byte), len(tracks))
			var stream *stream

			for _, track := range tracks {
				var medi *media.Media

				switch tcodec := track.Codec.(type) {
				case *mpegts.CodecH264:
					medi = &media.Media{
						Type: media.TypeVideo,
						Formats: []formats.Format{&formats.H264{
							PayloadTyp:        96,
							PacketizationMode: 1,
						}},
					}

					mediaCallbacks[track.ES.ElementaryPID] = func(pts time.Duration, data []byte) {
						au, err := h264.AnnexBUnmarshal(data)
						if err != nil {
							s.Log(logger.Warn, "%v", err)
							return
						}

						err = stream.writeData(medi, medi.Formats[0], &formatprocessor.UnitH264{
							PTS: pts,
							AU:  au,
							NTP: time.Now(),
						})
						if err != nil {
							s.Log(logger.Warn, "%v", err)
						}
					}

				case *mpegts.CodecH265:
					medi = &media.Media{
						Type: media.TypeVideo,
						Formats: []formats.Format{&formats.H265{
							PayloadTyp: 96,
						}},
					}

					mediaCallbacks[track.ES.ElementaryPID] = func(pts time.Duration, data []byte) {
						au, err := h264.AnnexBUnmarshal(data)
						if err != nil {
							s.Log(logger.Warn, "%v", err)
							return
						}

						err = stream.writeData(medi, medi.Formats[0], &formatprocessor.UnitH265{
							PTS: pts,
							AU:  au,
							NTP: time.Now(),
						})
						if err != nil {
							s.Log(logger.Warn, "%v", err)
						}
					}

				case *mpegts.CodecMPEG4Audio:
					medi = &media.Media{
						Type: media.TypeAudio,
						Formats: []formats.Format{&formats.MPEG4Audio{
							PayloadTyp:       96,
							SizeLength:       13,
							IndexLength:      3,
							IndexDeltaLength: 3,
							Config:           &tcodec.Config,
						}},
					}

					mediaCallbacks[track.ES.ElementaryPID] = func(pts time.Duration, data []byte) {
						var pkts mpeg4audio.ADTSPackets
						err := pkts.Unmarshal(data)
						if err != nil {
							s.Log(logger.Warn, "%v", err)
							return
						}

						aus := make([][]byte, len(pkts))
						for i, pkt := range pkts {
							aus[i] = pkt.AU
						}

						err = stream.writeData(medi, medi.Formats[0], &formatprocessor.UnitMPEG4Audio{
							PTS: pts,
							AUs: aus,
							NTP: time.Now(),
						})
						if err != nil {
							s.Log(logger.Warn, "%v", err)
						}
					}

				case *mpegts.CodecOpus:
					medi = &media.Media{
						Type: media.TypeAudio,
						Formats: []formats.Format{&formats.Opus{
							PayloadTyp: 96,
							IsStereo:   (tcodec.Channels == 2),
						}},
					}

					mediaCallbacks[track.ES.ElementaryPID] = func(pts time.Duration, data []byte) {
						pos := 0

						for {
							var au mpegts.OpusAccessUnit
							n, err := au.Unmarshal(data[pos:])
							if err != nil {
								s.Log(logger.Warn, "%v", err)
								return
							}
							pos += n

							err = stream.writeData(medi, medi.Formats[0], &formatprocessor.UnitOpus{
								PTS:   pts,
								Frame: au.Frame,
								NTP:   time.Now(),
							})
							if err != nil {
								s.Log(logger.Warn, "%v", err)
							}

							if len(data[pos:]) == 0 {
								break
							}

							pts += opusGetPacketDuration(au.Frame)
						}
					}
				}

				medias = append(medias, medi)
			}

			res := s.parent.sourceStaticImplSetReady(pathSourceStaticSetReadyReq{
				medias:             medias,
				generateRTPPackets: true,
			})
			if res.err != nil {
				return res.err
			}

			defer func() {
				s.parent.sourceStaticImplSetNotReady(pathSourceStaticSetNotReadyReq{})
			}()

			s.Log(logger.Info, "ready: %s", sourceMediaInfo(medias))

			stream = res.stream
			var timedec *mpegts.TimeDecoder

			for {
				pc.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
				data, err := dem.NextData()
				if err != nil {
					return err
				}

				if data.PES == nil {
					continue
				}

				if data.PES.Header.OptionalHeader == nil ||
					data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorNoPTSOrDTS ||
					data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorIsForbidden {
					return fmt.Errorf("PTS is missing")
				}

				var pts time.Duration
				if timedec == nil {
					timedec = mpegts.NewTimeDecoder(data.PES.Header.OptionalHeader.PTS.Base)
					pts = 0
				} else {
					pts = timedec.Decode(data.PES.Header.OptionalHeader.PTS.Base)
				}

				cb, ok := mediaCallbacks[data.PID]
				if !ok {
					continue
				}

				cb(pts, data.PES.Data)
			}
		}()
	}()

	select {
	case err := <-readerErr:
		return err

	case <-ctx.Done():
		pc.Close()
		<-readerErr
		return fmt.Errorf("terminated")
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*udpSource) apiSourceDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"udpSource"}
}

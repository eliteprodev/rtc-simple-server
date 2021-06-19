package rtspsession

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/readpublisher"
	"github.com/aler9/rtsp-simple-server/internal/rtspconn"
)

const (
	pauseAfterAuthError = 2 * time.Second
)

// PathMan is implemented by pathman.PathMan.
type PathMan interface {
	OnReadPublisherSetupPlay(readpublisher.SetupPlayReq)
	OnReadPublisherAnnounce(readpublisher.AnnounceReq)
}

// Parent is implemented by rtspserver.Server.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// Session is a RTSP server-side session.
type Session struct {
	rtspAddress string
	protocols   map[conf.Protocol]struct{}
	visualID    string
	ss          *gortsplib.ServerSession
	pathMan     PathMan
	parent      Parent

	path           readpublisher.Path
	setuppedTracks map[int]*gortsplib.Track // read
	onReadCmd      *externalcmd.Cmd         // read
	onPublishCmd   *externalcmd.Cmd         // publish
}

// New allocates a Session.
func New(
	rtspAddress string,
	protocols map[conf.Protocol]struct{},
	visualID string,
	ss *gortsplib.ServerSession,
	sc *gortsplib.ServerConn,
	pathMan PathMan,
	parent Parent) *Session {
	s := &Session{
		rtspAddress: rtspAddress,
		protocols:   protocols,
		visualID:    visualID,
		ss:          ss,
		pathMan:     pathMan,
		parent:      parent,
	}

	s.log(logger.Info, "opened by %v", sc.NetConn().RemoteAddr())

	return s
}

// ParentClose closes a Session.
func (s *Session) ParentClose() {
	switch s.ss.State() {
	case gortsplib.ServerSessionStatePlay:
		if s.onReadCmd != nil {
			s.onReadCmd.Close()
		}

	case gortsplib.ServerSessionStateRecord:
		if s.onPublishCmd != nil {
			s.onPublishCmd.Close()
		}
	}

	if s.path != nil {
		res := make(chan struct{})
		s.path.OnReadPublisherRemove(readpublisher.RemoveReq{s, res}) //nolint:govet
		<-res
		s.path = nil
	}

	s.log(logger.Info, "closed")
}

// Close closes a Session.
func (s *Session) Close() {
	s.ss.Close()
}

// IsReadPublisher implements readpublisher.ReadPublisher.
func (s *Session) IsReadPublisher() {}

// IsSource implements source.Source.
func (s *Session) IsSource() {}

// IsRTSPSession implements path.rtspSession.
func (s *Session) IsRTSPSession() {}

// VisualID returns the visual ID of the session.
func (s *Session) VisualID() string {
	return s.visualID
}

func (s *Session) displayedProtocol() string {
	if *s.ss.SetuppedDelivery() == base.StreamDeliveryMulticast {
		return "UDP-multicast"
	}
	return s.ss.SetuppedProtocol().String()
}

func (s *Session) log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[session %s] "+format, append([]interface{}{s.visualID}, args...)...)
}

// OnAnnounce is called by rtspserver.Server.
func (s *Session) OnAnnounce(c *rtspconn.Conn, ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	resc := make(chan readpublisher.AnnounceRes)
	s.pathMan.OnReadPublisherAnnounce(readpublisher.AnnounceReq{
		Author:   s,
		PathName: ctx.Path,
		Tracks:   ctx.Tracks,
		IP:       ctx.Conn.NetConn().RemoteAddr().(*net.TCPAddr).IP,
		ValidateCredentials: func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error {
			return c.ValidateCredentials(authMethods, pathUser, pathPass, ctx.Path, ctx.Req)
		},
		Res: resc,
	})
	res := <-resc

	if res.Err != nil {
		switch terr := res.Err.(type) {
		case readpublisher.ErrAuthNotCritical:
			return terr.Response, nil

		case readpublisher.ErrAuthCritical:
			// wait some seconds to stop brute force attacks
			<-time.After(pauseAfterAuthError)

			return terr.Response, errors.New(terr.Message)

		default:
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, res.Err
		}
	}

	s.path = res.Path

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// OnSetup is called by rtspserver.Server.
func (s *Session) OnSetup(c *rtspconn.Conn, ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	if ctx.Transport.Protocol == base.StreamProtocolUDP {
		if _, ok := s.protocols[conf.ProtocolUDP]; !ok {
			return &base.Response{
				StatusCode: base.StatusUnsupportedTransport,
			}, nil, nil
		}

		if ctx.Transport.Delivery != nil && *ctx.Transport.Delivery == base.StreamDeliveryMulticast {
			if _, ok := s.protocols[conf.ProtocolMulticast]; !ok {
				return &base.Response{
					StatusCode: base.StatusUnsupportedTransport,
				}, nil, nil
			}
		}
	} else if _, ok := s.protocols[conf.ProtocolTCP]; !ok {
		return &base.Response{
			StatusCode: base.StatusUnsupportedTransport,
		}, nil, nil
	}

	switch s.ss.State() {
	case gortsplib.ServerSessionStateInitial, gortsplib.ServerSessionStatePrePlay: // play
		resc := make(chan readpublisher.SetupPlayRes)
		s.pathMan.OnReadPublisherSetupPlay(readpublisher.SetupPlayReq{
			Author:   s,
			PathName: ctx.Path,
			IP:       ctx.Conn.NetConn().RemoteAddr().(*net.TCPAddr).IP,
			ValidateCredentials: func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error {
				return c.ValidateCredentials(authMethods, pathUser, pathPass, ctx.Path, ctx.Req)
			},
			Res: resc,
		})
		res := <-resc

		if res.Err != nil {
			switch terr := res.Err.(type) {
			case readpublisher.ErrAuthNotCritical:
				return terr.Response, nil, nil

			case readpublisher.ErrAuthCritical:
				// wait some seconds to stop brute force attacks
				<-time.After(pauseAfterAuthError)

				return terr.Response, nil, errors.New(terr.Message)

			case readpublisher.ErrNoOnePublishing:
				return &base.Response{
					StatusCode: base.StatusNotFound,
				}, nil, res.Err

			default:
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, nil, res.Err
			}
		}

		s.path = res.Path

		if ctx.TrackID >= len(res.Stream.Tracks()) {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, nil, fmt.Errorf("track %d does not exist", ctx.TrackID)
		}

		if s.setuppedTracks == nil {
			s.setuppedTracks = make(map[int]*gortsplib.Track)
		}
		s.setuppedTracks[ctx.TrackID] = res.Stream.Tracks()[ctx.TrackID]

		return &base.Response{
			StatusCode: base.StatusOK,
		}, res.Stream, nil

	default: // record
		return &base.Response{
			StatusCode: base.StatusOK,
		}, nil, nil
	}
}

// OnPlay is called by rtspserver.Server.
func (s *Session) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	h := make(base.Header)

	if s.ss.State() == gortsplib.ServerSessionStatePrePlay {
		if ctx.Path != s.path.Name() {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("path has changed, was '%s', now is '%s'", s.path.Name(), ctx.Path)
		}

		resc := make(chan readpublisher.PlayRes)
		s.path.OnReadPublisherPlay(readpublisher.PlayReq{s, resc}) //nolint:govet
		<-resc

		tracksLen := len(s.ss.SetuppedTracks())

		s.log(logger.Info, "is reading from path '%s', %d %s with %s",
			s.path.Name(),
			tracksLen,
			func() string {
				if tracksLen == 1 {
					return "track"
				}
				return "tracks"
			}(),
			s.displayedProtocol())

		if s.path.Conf().RunOnRead != "" {
			_, port, _ := net.SplitHostPort(s.rtspAddress)
			s.onReadCmd = externalcmd.New(s.path.Conf().RunOnRead, s.path.Conf().RunOnReadRestart, externalcmd.Environment{
				Path: s.path.Name(),
				Port: port,
			})
		}
	}

	return &base.Response{
		StatusCode: base.StatusOK,
		Header:     h,
	}, nil
}

// OnRecord is called by rtspserver.Server.
func (s *Session) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	if ctx.Path != s.path.Name() {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, fmt.Errorf("path has changed, was '%s', now is '%s'", s.path.Name(), ctx.Path)
	}

	resc := make(chan readpublisher.RecordRes)
	s.path.OnReadPublisherRecord(readpublisher.RecordReq{Author: s, Res: resc})
	res := <-resc

	if res.Err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, res.Err
	}

	tracksLen := len(s.ss.AnnouncedTracks())

	s.log(logger.Info, "is publishing to path '%s', %d %s with %s",
		s.path.Name(),
		tracksLen,
		func() string {
			if tracksLen == 1 {
				return "track"
			}
			return "tracks"
		}(),
		s.displayedProtocol())

	if s.path.Conf().RunOnPublish != "" {
		_, port, _ := net.SplitHostPort(s.rtspAddress)
		s.onPublishCmd = externalcmd.New(s.path.Conf().RunOnPublish, s.path.Conf().RunOnPublishRestart, externalcmd.Environment{
			Path: s.path.Name(),
			Port: port,
		})
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// OnPause is called by rtspserver.Server.
func (s *Session) OnPause(ctx *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	switch s.ss.State() {
	case gortsplib.ServerSessionStatePlay:
		if s.onReadCmd != nil {
			s.onReadCmd.Close()
		}

		res := make(chan struct{})
		s.path.OnReadPublisherPause(readpublisher.PauseReq{s, res}) //nolint:govet
		<-res

	case gortsplib.ServerSessionStateRecord:
		if s.onPublishCmd != nil {
			s.onPublishCmd.Close()
		}

		res := make(chan struct{})
		s.path.OnReadPublisherPause(readpublisher.PauseReq{s, res}) //nolint:govet
		<-res
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// OnFrame implements path.Reader.
func (s *Session) OnFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	s.ss.WriteFrame(trackID, streamType, payload)
}

// OnIncomingFrame is called by rtspserver.Server.
func (s *Session) OnIncomingFrame(ctx *gortsplib.ServerHandlerOnFrameCtx) {
	if s.ss.State() != gortsplib.ServerSessionStateRecord {
		return
	}

	s.path.OnFrame(ctx.TrackID, ctx.StreamType, ctx.Payload)
}

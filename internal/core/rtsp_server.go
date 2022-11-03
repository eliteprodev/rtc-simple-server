package core

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"
	"github.com/aler9/gortsplib/pkg/liberrors"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type rtspServerAPISessionsListItem struct {
	Created    time.Time `json:"created"`
	RemoteAddr string    `json:"remoteAddr"`
	State      string    `json:"state"`
}

type rtspServerAPISessionsListData struct {
	Items map[string]rtspServerAPISessionsListItem `json:"items"`
}

type rtspServerAPISessionsListRes struct {
	data *rtspServerAPISessionsListData
	err  error
}

type rtspServerAPISessionsListReq struct{}

type rtspServerAPISessionsKickRes struct {
	err error
}

type rtspServerAPISessionsKickReq struct {
	id string
}

type rtspServerParent interface {
	Log(logger.Level, string, ...interface{})
}

func printAddresses(srv *gortsplib.Server) string {
	var ret []string

	ret = append(ret, fmt.Sprintf("%s (TCP)", srv.RTSPAddress))

	if srv.UDPRTPAddress != "" {
		ret = append(ret, fmt.Sprintf("%s (UDP/RTP)", srv.UDPRTPAddress))
	}

	if srv.UDPRTCPAddress != "" {
		ret = append(ret, fmt.Sprintf("%s (UDP/RTCP)", srv.UDPRTCPAddress))
	}

	return strings.Join(ret, ", ")
}

type rtspServer struct {
	externalAuthenticationURL string
	authMethods               []headers.AuthMethod
	readTimeout               conf.StringDuration
	isTLS                     bool
	rtspAddress               string
	protocols                 map[conf.Protocol]struct{}
	runOnConnect              string
	runOnConnectRestart       bool
	externalCmdPool           *externalcmd.Pool
	metrics                   *metrics
	pathManager               *pathManager
	parent                    rtspServerParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	srv       *gortsplib.Server
	mutex     sync.RWMutex
	conns     map[*gortsplib.ServerConn]*rtspConn
	sessions  map[*gortsplib.ServerSession]*rtspSession
}

func newRTSPServer(
	parentCtx context.Context,
	externalAuthenticationURL string,
	address string,
	authMethods []headers.AuthMethod,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	useUDP bool,
	useMulticast bool,
	rtpAddress string,
	rtcpAddress string,
	multicastIPRange string,
	multicastRTPPort int,
	multicastRTCPPort int,
	isTLS bool,
	serverCert string,
	serverKey string,
	rtspAddress string,
	protocols map[conf.Protocol]struct{},
	runOnConnect string,
	runOnConnectRestart bool,
	externalCmdPool *externalcmd.Pool,
	metrics *metrics,
	pathManager *pathManager,
	parent rtspServerParent,
) (*rtspServer, error) {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &rtspServer{
		externalAuthenticationURL: externalAuthenticationURL,
		authMethods:               authMethods,
		readTimeout:               readTimeout,
		isTLS:                     isTLS,
		rtspAddress:               rtspAddress,
		protocols:                 protocols,
		externalCmdPool:           externalCmdPool,
		metrics:                   metrics,
		pathManager:               pathManager,
		parent:                    parent,
		ctx:                       ctx,
		ctxCancel:                 ctxCancel,
		conns:                     make(map[*gortsplib.ServerConn]*rtspConn),
		sessions:                  make(map[*gortsplib.ServerSession]*rtspSession),
	}

	s.srv = &gortsplib.Server{
		Handler:          s,
		ReadTimeout:      time.Duration(readTimeout),
		WriteTimeout:     time.Duration(writeTimeout),
		ReadBufferCount:  readBufferCount,
		WriteBufferCount: readBufferCount,
		RTSPAddress:      address,
	}

	if useUDP {
		s.srv.UDPRTPAddress = rtpAddress
		s.srv.UDPRTCPAddress = rtcpAddress
	}

	if useMulticast {
		s.srv.MulticastIPRange = multicastIPRange
		s.srv.MulticastRTPPort = multicastRTPPort
		s.srv.MulticastRTCPPort = multicastRTCPPort
	}

	if isTLS {
		cert, err := tls.LoadX509KeyPair(serverCert, serverKey)
		if err != nil {
			return nil, err
		}

		s.srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

	err := s.srv.Start()
	if err != nil {
		return nil, err
	}

	s.log(logger.Info, "listener opened on %s", printAddresses(s.srv))

	if s.metrics != nil {
		if !isTLS {
			s.metrics.rtspServerSet(s)
		} else {
			s.metrics.rtspsServerSet(s)
		}
	}

	s.wg.Add(1)
	go s.run()

	return s, nil
}

func (s *rtspServer) log(level logger.Level, format string, args ...interface{}) {
	label := func() string {
		if s.isTLS {
			return "RTSPS"
		}
		return "RTSP"
	}()
	s.parent.Log(level, "[%s] "+format, append([]interface{}{label}, args...)...)
}

func (s *rtspServer) close() {
	s.log(logger.Info, "listener is closing")
	s.ctxCancel()
	s.wg.Wait()
}

func (s *rtspServer) run() {
	defer s.wg.Done()

	serverErr := make(chan error)
	go func() {
		serverErr <- s.srv.Wait()
	}()

outer:
	select {
	case err := <-serverErr:
		s.log(logger.Error, "%s", err)
		break outer

	case <-s.ctx.Done():
		s.srv.Close()
		<-serverErr
		break outer
	}

	s.ctxCancel()

	if s.metrics != nil {
		if !s.isTLS {
			s.metrics.rtspServerSet(nil)
		} else {
			s.metrics.rtspsServerSet(nil)
		}
	}
}

func (s *rtspServer) newSessionID() (string, error) {
	for {
		b := make([]byte, 4)
		_, err := rand.Read(b)
		if err != nil {
			return "", err
		}

		u := uint32(b[3])<<24 | uint32(b[2])<<16 | uint32(b[1])<<8 | uint32(b[0])
		u %= 899999999
		u += 100000000

		id := strconv.FormatUint(uint64(u), 10)

		alreadyPresent := func() bool {
			for _, s := range s.sessions {
				if s.id == id {
					return true
				}
			}
			return false
		}()
		if !alreadyPresent {
			return id, nil
		}
	}
}

// OnConnOpen implements gortsplib.ServerHandlerOnConnOpen.
func (s *rtspServer) OnConnOpen(ctx *gortsplib.ServerHandlerOnConnOpenCtx) {
	c := newRTSPConn(
		s.externalAuthenticationURL,
		s.rtspAddress,
		s.authMethods,
		s.readTimeout,
		s.runOnConnect,
		s.runOnConnectRestart,
		s.externalCmdPool,
		s.pathManager,
		ctx.Conn,
		s)
	s.mutex.Lock()
	s.conns[ctx.Conn] = c
	s.mutex.Unlock()
	ctx.Conn.SetUserData(c)
}

// OnConnClose implements gortsplib.ServerHandlerOnConnClose.
func (s *rtspServer) OnConnClose(ctx *gortsplib.ServerHandlerOnConnCloseCtx) {
	s.mutex.Lock()
	c := s.conns[ctx.Conn]
	delete(s.conns, ctx.Conn)
	s.mutex.Unlock()
	c.onClose(ctx.Error)
}

// OnRequest implements gortsplib.ServerHandlerOnRequest.
func (s *rtspServer) OnRequest(sc *gortsplib.ServerConn, req *base.Request) {
	c := sc.UserData().(*rtspConn)
	c.onRequest(req)
}

// OnResponse implements gortsplib.ServerHandlerOnResponse.
func (s *rtspServer) OnResponse(sc *gortsplib.ServerConn, res *base.Response) {
	c := sc.UserData().(*rtspConn)
	c.OnResponse(res)
}

// OnSessionOpen implements gortsplib.ServerHandlerOnSessionOpen.
func (s *rtspServer) OnSessionOpen(ctx *gortsplib.ServerHandlerOnSessionOpenCtx) {
	s.mutex.Lock()
	id, _ := s.newSessionID()
	se := newRTSPSession(
		s.isTLS,
		s.protocols,
		id,
		ctx.Session,
		ctx.Conn,
		s.externalCmdPool,
		s.pathManager,
		s)
	s.sessions[ctx.Session] = se
	s.mutex.Unlock()
	ctx.Session.SetUserData(se)
}

// OnSessionClose implements gortsplib.ServerHandlerOnSessionClose.
func (s *rtspServer) OnSessionClose(ctx *gortsplib.ServerHandlerOnSessionCloseCtx) {
	s.mutex.Lock()
	se := s.sessions[ctx.Session]
	delete(s.sessions, ctx.Session)
	s.mutex.Unlock()

	if se != nil {
		se.onClose(ctx.Error)
	}
}

// OnDescribe implements gortsplib.ServerHandlerOnDescribe.
func (s *rtspServer) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	c := ctx.Conn.UserData().(*rtspConn)
	return c.onDescribe(ctx)
}

// OnAnnounce implements gortsplib.ServerHandlerOnAnnounce.
func (s *rtspServer) OnAnnounce(ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	c := ctx.Conn.UserData().(*rtspConn)
	se := ctx.Session.UserData().(*rtspSession)
	return se.onAnnounce(c, ctx)
}

// OnSetup implements gortsplib.ServerHandlerOnSetup.
func (s *rtspServer) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	c := ctx.Conn.UserData().(*rtspConn)
	se := ctx.Session.UserData().(*rtspSession)
	return se.onSetup(c, ctx)
}

// OnPlay implements gortsplib.ServerHandlerOnPlay.
func (s *rtspServer) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	se := ctx.Session.UserData().(*rtspSession)
	return se.onPlay(ctx)
}

// OnRecord implements gortsplib.ServerHandlerOnRecord.
func (s *rtspServer) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	se := ctx.Session.UserData().(*rtspSession)
	return se.onRecord(ctx)
}

// OnPause implements gortsplib.ServerHandlerOnPause.
func (s *rtspServer) OnPause(ctx *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	se := ctx.Session.UserData().(*rtspSession)
	return se.onPause(ctx)
}

// OnPacketRTP implements gortsplib.ServerHandlerOnPacketRTP.
func (s *rtspServer) OnPacketRTP(ctx *gortsplib.ServerHandlerOnPacketRTPCtx) {
	se := ctx.Session.UserData().(*rtspSession)
	se.onPacketRTP(ctx)
}

// OnDecodeError implements gortsplib.ServerHandlerOnOnDecodeError.
func (s *rtspServer) OnDecodeError(ctx *gortsplib.ServerHandlerOnDecodeErrorCtx) {
	se := ctx.Session.UserData().(*rtspSession)
	se.onDecodeError(ctx)
}

// apiSessionsList is called by api and metrics.
func (s *rtspServer) apiSessionsList(req rtspServerAPISessionsListReq) rtspServerAPISessionsListRes {
	select {
	case <-s.ctx.Done():
		return rtspServerAPISessionsListRes{err: fmt.Errorf("terminated")}
	default:
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	data := &rtspServerAPISessionsListData{
		Items: make(map[string]rtspServerAPISessionsListItem),
	}

	for _, s := range s.sessions {
		data.Items[s.id] = rtspServerAPISessionsListItem{
			Created:    s.created,
			RemoteAddr: s.remoteAddr().String(),
			State: func() string {
				switch s.safeState() {
				case gortsplib.ServerSessionStatePrePlay,
					gortsplib.ServerSessionStatePlay:
					return "read"

				case gortsplib.ServerSessionStatePreRecord,
					gortsplib.ServerSessionStateRecord:
					return "publish"
				}
				return "idle"
			}(),
		}
	}

	return rtspServerAPISessionsListRes{data: data}
}

// apiSessionsKick is called by api.
func (s *rtspServer) apiSessionsKick(req rtspServerAPISessionsKickReq) rtspServerAPISessionsKickRes {
	select {
	case <-s.ctx.Done():
		return rtspServerAPISessionsKickRes{err: fmt.Errorf("terminated")}
	default:
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for key, se := range s.sessions {
		if se.id == req.id {
			se.close()
			delete(s.sessions, key)
			se.onClose(liberrors.ErrServerTerminated{})
			return rtspServerAPISessionsKickRes{}
		}
	}

	return rtspServerAPISessionsKickRes{err: fmt.Errorf("not found")}
}

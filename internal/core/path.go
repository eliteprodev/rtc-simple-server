package core

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func newEmptyTimer() *time.Timer {
	t := time.NewTimer(0)
	<-t.C
	return t
}

type authenticateFunc func(
	pathIPs []interface{},
	pathUser conf.Credential,
	pathPass conf.Credential,
) error

type pathErrNoOnePublishing struct {
	pathName string
}

// Error implements the error interface.
func (e pathErrNoOnePublishing) Error() string {
	return fmt.Sprintf("no one is publishing to path '%s'", e.pathName)
}

type pathErrAuthNotCritical struct {
	message  string
	response *base.Response
}

// Error implements the error interface.
func (pathErrAuthNotCritical) Error() string {
	return "non-critical authentication error"
}

type pathErrAuthCritical struct {
	message  string
	response *base.Response
}

// Error implements the error interface.
func (pathErrAuthCritical) Error() string {
	return "critical authentication error"
}

type pathParent interface {
	log(logger.Level, string, ...interface{})
	onPathSourceReady(*path)
	onPathClose(*path)
}

type pathRTSPSession interface {
	IsRTSPSession()
}

type sourceRedirect struct{}

// onSourceAPIDescribe implements source.
func (*sourceRedirect) onSourceAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"redirect"}
}

type pathReaderState int

const (
	pathReaderStatePrePlay pathReaderState = iota
	pathReaderStatePlay
)

type pathOnDemandState int

const (
	pathOnDemandStateInitial pathOnDemandState = iota
	pathOnDemandStateWaitingReady
	pathOnDemandStateReady
	pathOnDemandStateClosing
)

type pathSourceStaticSetReadyRes struct {
	stream *stream
	err    error
}

type pathSourceStaticSetReadyReq struct {
	source sourceStatic
	tracks gortsplib.Tracks
	res    chan pathSourceStaticSetReadyRes
}

type pathSourceStaticSetNotReadyReq struct {
	source sourceStatic
	res    chan struct{}
}

type pathReaderRemoveReq struct {
	author reader
	res    chan struct{}
}

type pathPublisherRemoveReq struct {
	author publisher
	res    chan struct{}
}

type pathDescribeRes struct {
	path     *path
	stream   *stream
	redirect string
	err      error
}

type pathDescribeReq struct {
	pathName     string
	url          *base.URL
	authenticate authenticateFunc
	res          chan pathDescribeRes
}

type pathReaderSetupPlayRes struct {
	path   *path
	stream *stream
	err    error
}

type pathReaderSetupPlayReq struct {
	author       reader
	pathName     string
	authenticate authenticateFunc
	res          chan pathReaderSetupPlayRes
}

type pathPublisherAnnounceRes struct {
	path *path
	err  error
}

type pathPublisherAnnounceReq struct {
	author       publisher
	pathName     string
	authenticate authenticateFunc
	res          chan pathPublisherAnnounceRes
}

type pathReaderPlayReq struct {
	author reader
	res    chan struct{}
}

type pathPublisherRecordRes struct {
	stream *stream
	err    error
}

type pathPublisherRecordReq struct {
	author publisher
	tracks gortsplib.Tracks
	res    chan pathPublisherRecordRes
}

type pathReaderPauseReq struct {
	author reader
	res    chan struct{}
}

type pathPublisherPauseReq struct {
	author publisher
	res    chan struct{}
}

type pathAPIPathsListItem struct {
	ConfName    string         `json:"confName"`
	Conf        *conf.PathConf `json:"conf"`
	Source      interface{}    `json:"source"`
	SourceReady bool           `json:"sourceReady"`
	Readers     []interface{}  `json:"readers"`
}

type pathAPIPathsListData struct {
	Items map[string]pathAPIPathsListItem `json:"items"`
}

type pathAPIPathsListRes struct {
	data  *pathAPIPathsListData
	paths map[string]*path
	err   error
}

type pathAPIPathsListReq struct {
	res chan pathAPIPathsListRes
}

type pathAPIPathsListSubReq struct {
	data *pathAPIPathsListData
	res  chan struct{}
}

type path struct {
	rtspAddress     string
	readTimeout     conf.StringDuration
	writeTimeout    conf.StringDuration
	readBufferCount int
	readBufferSize  int
	confName        string
	conf            *conf.PathConf
	name            string
	matches         []string
	wg              *sync.WaitGroup
	externalCmdPool *externalcmd.Pool
	parent          pathParent

	ctx                context.Context
	ctxCancel          func()
	source             source
	sourceReady        bool
	sourceStaticWg     sync.WaitGroup
	readers            map[reader]pathReaderState
	describeRequests   []pathDescribeReq
	setupPlayRequests  []pathReaderSetupPlayReq
	stream             *stream
	onDemandCmd        *externalcmd.Cmd
	onReadyCmd         *externalcmd.Cmd
	onDemandReadyTimer *time.Timer
	onDemandCloseTimer *time.Timer
	onDemandState      pathOnDemandState

	// in
	sourceStaticSetReady    chan pathSourceStaticSetReadyReq
	sourceStaticSetNotReady chan pathSourceStaticSetNotReadyReq
	describe                chan pathDescribeReq
	publisherRemove         chan pathPublisherRemoveReq
	publisherAnnounce       chan pathPublisherAnnounceReq
	publisherRecord         chan pathPublisherRecordReq
	publisherPause          chan pathPublisherPauseReq
	readerRemove            chan pathReaderRemoveReq
	readerSetupPlay         chan pathReaderSetupPlayReq
	readerPlay              chan pathReaderPlayReq
	readerPause             chan pathReaderPauseReq
	apiPathsList            chan pathAPIPathsListSubReq
}

func newPath(
	parentCtx context.Context,
	rtspAddress string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	readBufferSize int,
	confName string,
	conf *conf.PathConf,
	name string,
	matches []string,
	wg *sync.WaitGroup,
	externalCmdPool *externalcmd.Pool,
	parent pathParent) *path {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	pa := &path{
		rtspAddress:             rtspAddress,
		readTimeout:             readTimeout,
		writeTimeout:            writeTimeout,
		readBufferCount:         readBufferCount,
		readBufferSize:          readBufferSize,
		confName:                confName,
		conf:                    conf,
		name:                    name,
		matches:                 matches,
		wg:                      wg,
		externalCmdPool:         externalCmdPool,
		parent:                  parent,
		ctx:                     ctx,
		ctxCancel:               ctxCancel,
		readers:                 make(map[reader]pathReaderState),
		onDemandReadyTimer:      newEmptyTimer(),
		onDemandCloseTimer:      newEmptyTimer(),
		sourceStaticSetReady:    make(chan pathSourceStaticSetReadyReq),
		sourceStaticSetNotReady: make(chan pathSourceStaticSetNotReadyReq),
		describe:                make(chan pathDescribeReq),
		publisherRemove:         make(chan pathPublisherRemoveReq),
		publisherAnnounce:       make(chan pathPublisherAnnounceReq),
		publisherRecord:         make(chan pathPublisherRecordReq),
		publisherPause:          make(chan pathPublisherPauseReq),
		readerRemove:            make(chan pathReaderRemoveReq),
		readerSetupPlay:         make(chan pathReaderSetupPlayReq),
		readerPlay:              make(chan pathReaderPlayReq),
		readerPause:             make(chan pathReaderPauseReq),
		apiPathsList:            make(chan pathAPIPathsListSubReq),
	}

	pa.log(logger.Debug, "created")

	pa.wg.Add(1)
	go pa.run()

	return pa
}

func (pa *path) close() {
	pa.ctxCancel()
}

// Log is the main logging function.
func (pa *path) log(level logger.Level, format string, args ...interface{}) {
	pa.parent.log(level, "[path "+pa.name+"] "+format, args...)
}

// ConfName returns the configuration name of this path.
func (pa *path) ConfName() string {
	return pa.confName
}

// Conf returns the configuration of this path.
func (pa *path) Conf() *conf.PathConf {
	return pa.conf
}

// Name returns the name of this path.
func (pa *path) Name() string {
	return pa.name
}

func (pa *path) run() {
	defer pa.wg.Done()

	if pa.conf.Source == "redirect" {
		pa.source = &sourceRedirect{}
	} else if !pa.conf.SourceOnDemand && pa.hasStaticSource() {
		pa.staticSourceCreate()
	}

	var onInitCmd *externalcmd.Cmd
	if pa.conf.RunOnInit != "" {
		pa.log(logger.Info, "runOnInit command started")
		onInitCmd = externalcmd.NewCmd(
			pa.externalCmdPool,
			pa.conf.RunOnInit,
			pa.conf.RunOnInitRestart,
			pa.externalCmdEnv(),
			func(co int) {
				pa.log(logger.Info, "runOnInit command exited with code %d", co)
			})
	}

	err := func() error {
		for {
			select {
			case <-pa.onDemandReadyTimer.C:
				for _, req := range pa.describeRequests {
					req.res <- pathDescribeRes{err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
				}
				pa.describeRequests = nil

				for _, req := range pa.setupPlayRequests {
					req.res <- pathReaderSetupPlayRes{err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
				}
				pa.setupPlayRequests = nil

				pa.onDemandCloseSource()

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case <-pa.onDemandCloseTimer.C:
				pa.onDemandCloseSource()

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.sourceStaticSetReady:
				if req.source == pa.source {
					pa.sourceSetReady(req.tracks)
					req.res <- pathSourceStaticSetReadyRes{stream: pa.stream}
				} else {
					req.res <- pathSourceStaticSetReadyRes{err: fmt.Errorf("terminated")}
				}

			case req := <-pa.sourceStaticSetNotReady:
				if req.source == pa.source {
					if pa.isOnDemand() && pa.onDemandState != pathOnDemandStateInitial {
						pa.onDemandCloseSource()
					} else {
						pa.sourceSetNotReady()
					}
				}
				close(req.res)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.describe:
				pa.handleDescribe(req)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.publisherRemove:
				pa.handlePublisherRemove(req)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.publisherAnnounce:
				pa.handlePublisherAnnounce(req)

			case req := <-pa.publisherRecord:
				pa.handlePublisherRecord(req)

			case req := <-pa.publisherPause:
				pa.handlePublisherPause(req)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.readerRemove:
				pa.handleReaderRemove(req)

			case req := <-pa.readerSetupPlay:
				pa.handleReaderSetupPlay(req)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.readerPlay:
				pa.handleReaderPlay(req)

			case req := <-pa.readerPause:
				pa.handleReaderPause(req)

			case req := <-pa.apiPathsList:
				pa.handleAPIPathsList(req)

			case <-pa.ctx.Done():
				return fmt.Errorf("terminated")
			}
		}
	}()

	pa.ctxCancel()

	pa.onDemandReadyTimer.Stop()
	pa.onDemandCloseTimer.Stop()

	if onInitCmd != nil {
		onInitCmd.Close()
		pa.log(logger.Info, "runOnInit command stopped")
	}

	for _, req := range pa.describeRequests {
		req.res <- pathDescribeRes{err: fmt.Errorf("terminated")}
	}

	for _, req := range pa.setupPlayRequests {
		req.res <- pathReaderSetupPlayRes{err: fmt.Errorf("terminated")}
	}

	pa.sourceSetNotReady()

	if pa.source != nil {
		if source, ok := pa.source.(sourceStatic); ok {
			source.close()
			pa.sourceStaticWg.Wait()
		} else if source, ok := pa.source.(publisher); ok {
			source.close()
		}
	}

	if pa.onDemandCmd != nil {
		pa.onDemandCmd.Close()
		pa.log(logger.Info, "runOnDemand command stopped")
	}

	pa.log(logger.Debug, "destroyed (%v)", err)

	pa.parent.onPathClose(pa)
}

func (pa *path) shouldClose() bool {
	return pa.conf.Regexp != nil &&
		pa.source == nil &&
		len(pa.readers) == 0 &&
		len(pa.describeRequests) == 0 &&
		len(pa.setupPlayRequests) == 0
}

func (pa *path) hasStaticSource() bool {
	return strings.HasPrefix(pa.conf.Source, "rtsp://") ||
		strings.HasPrefix(pa.conf.Source, "rtsps://") ||
		strings.HasPrefix(pa.conf.Source, "rtmp://") ||
		strings.HasPrefix(pa.conf.Source, "http://") ||
		strings.HasPrefix(pa.conf.Source, "https://")
}

func (pa *path) isOnDemand() bool {
	return (pa.hasStaticSource() && pa.conf.SourceOnDemand) || pa.conf.RunOnDemand != ""
}

func (pa *path) externalCmdEnv() externalcmd.Environment {
	_, port, _ := net.SplitHostPort(pa.rtspAddress)
	env := externalcmd.Environment{
		"RTSP_PATH": pa.name,
		"RTSP_PORT": port,
	}

	if len(pa.matches) > 1 {
		for i, ma := range pa.matches[1:] {
			env["G"+strconv.FormatInt(int64(i+1), 10)] = ma
		}
	}

	return env
}

func (pa *path) onDemandStartSource() {
	pa.onDemandReadyTimer.Stop()
	if pa.hasStaticSource() {
		pa.staticSourceCreate()
		pa.onDemandReadyTimer = time.NewTimer(time.Duration(pa.conf.SourceOnDemandStartTimeout))
	} else {
		pa.log(logger.Info, "runOnDemand command started")
		pa.onDemandCmd = externalcmd.NewCmd(
			pa.externalCmdPool,
			pa.conf.RunOnDemand,
			pa.conf.RunOnDemandRestart,
			pa.externalCmdEnv(),
			func(co int) {
				pa.log(logger.Info, "runOnDemand command exited with code %d", co)
			})
		pa.onDemandReadyTimer = time.NewTimer(time.Duration(pa.conf.RunOnDemandStartTimeout))
	}

	pa.onDemandState = pathOnDemandStateWaitingReady
}

func (pa *path) onDemandScheduleClose() {
	pa.onDemandCloseTimer.Stop()
	if pa.hasStaticSource() {
		pa.onDemandCloseTimer = time.NewTimer(time.Duration(pa.conf.SourceOnDemandCloseAfter))
	} else {
		pa.onDemandCloseTimer = time.NewTimer(time.Duration(pa.conf.RunOnDemandCloseAfter))
	}

	pa.onDemandState = pathOnDemandStateClosing
}

func (pa *path) onDemandCloseSource() {
	if pa.onDemandState == pathOnDemandStateClosing {
		pa.onDemandCloseTimer.Stop()
		pa.onDemandCloseTimer = newEmptyTimer()
	}

	// set state before doPublisherRemove()
	pa.onDemandState = pathOnDemandStateInitial

	if pa.hasStaticSource() {
		if pa.sourceReady {
			pa.sourceSetNotReady()
		}
		pa.source.(sourceStatic).close()
		pa.source = nil
	} else {
		if pa.source != nil {
			pa.source.(publisher).close()
			pa.doPublisherRemove()
		}

		if pa.onDemandCmd != nil {
			pa.onDemandCmd.Close()
			pa.onDemandCmd = nil
			pa.log(logger.Info, "runOnDemand command stopped")
		}
	}
}

func (pa *path) sourceSetReady(tracks gortsplib.Tracks) {
	pa.sourceReady = true
	pa.stream = newStream(tracks)

	if pa.isOnDemand() {
		pa.onDemandReadyTimer.Stop()
		pa.onDemandReadyTimer = newEmptyTimer()

		for _, req := range pa.describeRequests {
			req.res <- pathDescribeRes{
				stream: pa.stream,
			}
		}
		pa.describeRequests = nil

		for _, req := range pa.setupPlayRequests {
			pa.handleReaderSetupPlayPost(req)
		}
		pa.setupPlayRequests = nil

		if len(pa.readers) > 0 {
			pa.onDemandState = pathOnDemandStateReady
		} else {
			pa.onDemandScheduleClose()
		}
	}

	pa.parent.onPathSourceReady(pa)

	if pa.conf.RunOnReady != "" {
		pa.log(logger.Info, "runOnReady command started")
		pa.onReadyCmd = externalcmd.NewCmd(
			pa.externalCmdPool,
			pa.conf.RunOnReady,
			pa.conf.RunOnReadyRestart,
			pa.externalCmdEnv(),
			func(co int) {
				pa.log(logger.Info, "runOnReady command exited with code %d", co)
			})
	}
}

func (pa *path) sourceSetNotReady() {
	for r := range pa.readers {
		pa.doReaderRemove(r)
		r.close()
	}

	if pa.onReadyCmd != nil {
		pa.onReadyCmd.Close()
		pa.onReadyCmd = nil
		pa.log(logger.Info, "runOnReady command stopped")
	}

	pa.sourceReady = false

	if pa.stream != nil {
		pa.stream.close()
		pa.stream = nil
	}
}

func (pa *path) staticSourceCreate() {
	switch {
	case strings.HasPrefix(pa.conf.Source, "rtsp://") ||
		strings.HasPrefix(pa.conf.Source, "rtsps://"):
		pa.source = newRTSPSource(
			pa.ctx,
			pa.conf.Source,
			pa.conf.SourceProtocol,
			pa.conf.SourceAnyPortEnable,
			pa.conf.SourceFingerprint,
			pa.readTimeout,
			pa.writeTimeout,
			pa.readBufferCount,
			pa.readBufferSize,
			&pa.sourceStaticWg,
			pa)
	case strings.HasPrefix(pa.conf.Source, "rtmp://"):
		pa.source = newRTMPSource(
			pa.ctx,
			pa.conf.Source,
			pa.readTimeout,
			pa.writeTimeout,
			&pa.sourceStaticWg,
			pa)
	case strings.HasPrefix(pa.conf.Source, "http://") ||
		strings.HasPrefix(pa.conf.Source, "https://"):
		pa.source = newHLSSource(
			pa.ctx,
			pa.conf.Source,
			pa.conf.SourceFingerprint,
			&pa.sourceStaticWg,
			pa)
	}
}

func (pa *path) doReaderRemove(r reader) {
	state := pa.readers[r]

	if state == pathReaderStatePlay {
		pa.stream.readerRemove(r)
	}

	delete(pa.readers, r)
}

func (pa *path) doPublisherRemove() {
	if pa.sourceReady {
		if pa.isOnDemand() && pa.onDemandState != pathOnDemandStateInitial {
			pa.onDemandCloseSource()
		} else {
			pa.sourceSetNotReady()
		}
	}

	pa.source = nil
}

func (pa *path) handleDescribe(req pathDescribeReq) {
	if _, ok := pa.source.(*sourceRedirect); ok {
		req.res <- pathDescribeRes{
			redirect: pa.conf.SourceRedirect,
		}
		return
	}

	if pa.sourceReady {
		req.res <- pathDescribeRes{
			stream: pa.stream,
		}
		return
	}

	if pa.isOnDemand() {
		if pa.onDemandState == pathOnDemandStateInitial {
			pa.onDemandStartSource()
		}
		pa.describeRequests = append(pa.describeRequests, req)
		return
	}

	if pa.conf.Fallback != "" {
		fallbackURL := func() string {
			if strings.HasPrefix(pa.conf.Fallback, "/") {
				ur := base.URL{
					Scheme: req.url.Scheme,
					User:   req.url.User,
					Host:   req.url.Host,
					Path:   pa.conf.Fallback,
				}
				return ur.String()
			}
			return pa.conf.Fallback
		}()
		req.res <- pathDescribeRes{redirect: fallbackURL}
		return
	}

	req.res <- pathDescribeRes{err: pathErrNoOnePublishing{pathName: pa.name}}
}

func (pa *path) handlePublisherRemove(req pathPublisherRemoveReq) {
	if pa.source == req.author {
		pa.doPublisherRemove()
	}
	close(req.res)
}

func (pa *path) handlePublisherAnnounce(req pathPublisherAnnounceReq) {
	if pa.source != nil {
		if pa.hasStaticSource() {
			req.res <- pathPublisherAnnounceRes{err: fmt.Errorf("path '%s' is assigned to a static source", pa.name)}
			return
		}

		if pa.conf.DisablePublisherOverride {
			req.res <- pathPublisherAnnounceRes{err: fmt.Errorf("another publisher is already publishing to path '%s'", pa.name)}
			return
		}

		pa.log(logger.Info, "closing existing publisher")
		pa.source.(publisher).close()
		pa.doPublisherRemove()
	}

	pa.source = req.author

	req.res <- pathPublisherAnnounceRes{path: pa}
}

func (pa *path) handlePublisherRecord(req pathPublisherRecordReq) {
	if pa.source != req.author {
		req.res <- pathPublisherRecordRes{err: fmt.Errorf("publisher is not assigned to this path anymore")}
		return
	}

	req.author.onPublisherAccepted(len(req.tracks))

	pa.sourceSetReady(req.tracks)

	req.res <- pathPublisherRecordRes{stream: pa.stream}
}

func (pa *path) handlePublisherPause(req pathPublisherPauseReq) {
	if req.author == pa.source && pa.sourceReady {
		if pa.isOnDemand() && pa.onDemandState != pathOnDemandStateInitial {
			pa.onDemandCloseSource()
		} else {
			pa.sourceSetNotReady()
		}
	}
	close(req.res)
}

func (pa *path) handleReaderRemove(req pathReaderRemoveReq) {
	if _, ok := pa.readers[req.author]; ok {
		pa.doReaderRemove(req.author)
	}
	close(req.res)

	if pa.isOnDemand() &&
		len(pa.readers) == 0 &&
		pa.onDemandState == pathOnDemandStateReady {
		pa.onDemandScheduleClose()
	}
}

func (pa *path) handleReaderSetupPlay(req pathReaderSetupPlayReq) {
	if pa.sourceReady {
		pa.handleReaderSetupPlayPost(req)
		return
	}

	if pa.isOnDemand() {
		if pa.onDemandState == pathOnDemandStateInitial {
			pa.onDemandStartSource()
		}
		pa.setupPlayRequests = append(pa.setupPlayRequests, req)
		return
	}

	req.res <- pathReaderSetupPlayRes{err: pathErrNoOnePublishing{pathName: pa.name}}
}

func (pa *path) handleReaderSetupPlayPost(req pathReaderSetupPlayReq) {
	pa.readers[req.author] = pathReaderStatePrePlay

	if pa.isOnDemand() && pa.onDemandState == pathOnDemandStateClosing {
		pa.onDemandState = pathOnDemandStateReady
		pa.onDemandCloseTimer.Stop()
		pa.onDemandCloseTimer = newEmptyTimer()
	}

	req.res <- pathReaderSetupPlayRes{
		path:   pa,
		stream: pa.stream,
	}
}

func (pa *path) handleReaderPlay(req pathReaderPlayReq) {
	pa.readers[req.author] = pathReaderStatePlay

	pa.stream.readerAdd(req.author)

	req.author.onReaderAccepted()

	close(req.res)
}

func (pa *path) handleReaderPause(req pathReaderPauseReq) {
	if state, ok := pa.readers[req.author]; ok && state == pathReaderStatePlay {
		pa.readers[req.author] = pathReaderStatePrePlay
		pa.stream.readerRemove(req.author)
	}
	close(req.res)
}

func (pa *path) handleAPIPathsList(req pathAPIPathsListSubReq) {
	req.data.Items[pa.name] = pathAPIPathsListItem{
		ConfName: pa.confName,
		Conf:     pa.conf,
		Source: func() interface{} {
			if pa.source == nil {
				return nil
			}
			return pa.source.onSourceAPIDescribe()
		}(),
		SourceReady: pa.sourceReady,
		Readers: func() []interface{} {
			ret := []interface{}{}
			for r := range pa.readers {
				ret = append(ret, r.onReaderAPIDescribe())
			}
			return ret
		}(),
	}
	close(req.res)
}

// onSourceStaticSetReady is called by a sourceStatic.
func (pa *path) onSourceStaticSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes {
	req.res = make(chan pathSourceStaticSetReadyRes)
	select {
	case pa.sourceStaticSetReady <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathSourceStaticSetReadyRes{err: fmt.Errorf("terminated")}
	}
}

// onSourceStaticSetNotReady is called by a sourceStatic.
func (pa *path) onSourceStaticSetNotReady(req pathSourceStaticSetNotReadyReq) {
	req.res = make(chan struct{})
	select {
	case pa.sourceStaticSetNotReady <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// onDescribe is called by a reader or publisher through pathManager.
func (pa *path) onDescribe(req pathDescribeReq) pathDescribeRes {
	select {
	case pa.describe <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathDescribeRes{err: fmt.Errorf("terminated")}
	}
}

// onPublisherRemove is called by a publisher.
func (pa *path) onPublisherRemove(req pathPublisherRemoveReq) {
	req.res = make(chan struct{})
	select {
	case pa.publisherRemove <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// onPublisherAnnounce is called by a publisher through pathManager.
func (pa *path) onPublisherAnnounce(req pathPublisherAnnounceReq) pathPublisherAnnounceRes {
	select {
	case pa.publisherAnnounce <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathPublisherAnnounceRes{err: fmt.Errorf("terminated")}
	}
}

// onPublisherRecord is called by a publisher.
func (pa *path) onPublisherRecord(req pathPublisherRecordReq) pathPublisherRecordRes {
	req.res = make(chan pathPublisherRecordRes)
	select {
	case pa.publisherRecord <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathPublisherRecordRes{err: fmt.Errorf("terminated")}
	}
}

// onPublisherPause is called by a publisher.
func (pa *path) onPublisherPause(req pathPublisherPauseReq) {
	req.res = make(chan struct{})
	select {
	case pa.publisherPause <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// onReaderRemove is called by a reader.
func (pa *path) onReaderRemove(req pathReaderRemoveReq) {
	req.res = make(chan struct{})
	select {
	case pa.readerRemove <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// onReaderSetupPlay is called by a reader through pathManager.
func (pa *path) onReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes {
	select {
	case pa.readerSetupPlay <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathReaderSetupPlayRes{err: fmt.Errorf("terminated")}
	}
}

// onReaderPlay is called by a reader.
func (pa *path) onReaderPlay(req pathReaderPlayReq) {
	req.res = make(chan struct{})
	select {
	case pa.readerPlay <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// onReaderPause is called by a reader.
func (pa *path) onReaderPause(req pathReaderPauseReq) {
	req.res = make(chan struct{})
	select {
	case pa.readerPause <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// onAPIPathsList is called by api.
func (pa *path) onAPIPathsList(req pathAPIPathsListSubReq) {
	req.res = make(chan struct{})
	select {
	case pa.apiPathsList <- req:
		<-req.res

	case <-pa.ctx.Done():
	}
}

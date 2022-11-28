package core

import (
	"context"
	"time"

	"github.com/aler9/gortsplib"

	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rpicamera"
)

type rpiCameraSourceParent interface {
	log(logger.Level, string, ...interface{})
	sourceStaticImplSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	sourceStaticImplSetNotReady(req pathSourceStaticSetNotReadyReq)
}

type rpiCameraSource struct {
	params rpicamera.Params
	parent rpiCameraSourceParent
}

func newRPICameraSource(
	params rpicamera.Params,
	parent rpiCameraSourceParent,
) *rpiCameraSource {
	return &rpiCameraSource{
		params: params,
		parent: parent,
	}
}

func (s *rpiCameraSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[rpicamera source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *rpiCameraSource) run(ctx context.Context) error {
	track := &gortsplib.TrackH264{
		PayloadType:       96,
		PacketizationMode: 1,
	}
	tracks := gortsplib.Tracks{track}
	var stream *stream

	onData := func(dts time.Duration, nalus [][]byte) {
		if stream == nil {
			res := s.parent.sourceStaticImplSetReady(pathSourceStaticSetReadyReq{
				tracks:             tracks,
				generateRTPPackets: true,
			})
			if res.err != nil {
				return
			}

			s.Log(logger.Info, "ready: %s", sourceTrackInfo(tracks))
			stream = res.stream
		}

		err := stream.writeData(&dataH264{
			trackID: 0,
			pts:     dts,
			nalus:   nalus,
			ntp:     time.Now(),
		})
		if err != nil {
			s.Log(logger.Warn, "%v", err)
		}
	}

	cam, err := rpicamera.New(s.params, onData)
	if err != nil {
		return err
	}
	defer cam.Close()

	defer func() {
		if stream != nil {
			s.parent.sourceStaticImplSetNotReady(pathSourceStaticSetNotReadyReq{})
		}
	}()

	<-ctx.Done()
	return nil
}

// apiSourceDescribe implements sourceStaticImpl.
func (*rpiCameraSource) apiSourceDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"rpiCameraSource"}
}

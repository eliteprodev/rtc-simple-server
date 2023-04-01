package core

import (
	"context"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"

	"github.com/aler9/mediamtx/internal/conf"
	"github.com/aler9/mediamtx/internal/formatprocessor"
	"github.com/aler9/mediamtx/internal/logger"
)

type hlsSourceParent interface {
	log(logger.Level, string, ...interface{})
	sourceStaticImplSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	sourceStaticImplSetNotReady(req pathSourceStaticSetNotReadyReq)
}

type hlsSource struct {
	parent hlsSourceParent
}

func newHLSSource(
	parent hlsSourceParent,
) *hlsSource {
	return &hlsSource{
		parent: parent,
	}
}

func (s *hlsSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[hls source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *hlsSource) run(ctx context.Context, cnf *conf.PathConf, reloadConf chan *conf.PathConf) error {
	var stream *stream

	defer func() {
		if stream != nil {
			s.parent.sourceStaticImplSetNotReady(pathSourceStaticSetNotReadyReq{})
		}
	}()

	c := &gohlslib.Client{
		URI:         cnf.Source,
		Fingerprint: cnf.SourceFingerprint,
		Log: func(level gohlslib.LogLevel, format string, args ...interface{}) {
			s.Log(logger.Level(level), format, args...)
		},
	}

	c.OnTracks(func(tracks []*gohlslib.Track) error {
		var medias media.Medias

		for _, track := range tracks {
			var medi *media.Media

			switch tcodec := track.Codec.(type) {
			case *codecs.H264:
				medi = &media.Media{
					Type: media.TypeVideo,
					Formats: []formats.Format{&formats.H264{
						PayloadTyp:        96,
						PacketizationMode: 1,
						SPS:               tcodec.SPS,
						PPS:               tcodec.PPS,
					}},
				}

				c.OnData(track, func(pts time.Duration, unit interface{}) {
					err := stream.writeData(medi, medi.Formats[0], &formatprocessor.UnitH264{
						PTS: pts,
						AU:  unit.([][]byte),
						NTP: time.Now(),
					})
					if err != nil {
						s.Log(logger.Warn, "%v", err)
					}
				})

			case *codecs.H265:
				medi = &media.Media{
					Type: media.TypeVideo,
					Formats: []formats.Format{&formats.H265{
						PayloadTyp: 96,
						VPS:        tcodec.VPS,
						SPS:        tcodec.SPS,
						PPS:        tcodec.PPS,
					}},
				}

				c.OnData(track, func(pts time.Duration, unit interface{}) {
					err := stream.writeData(medi, medi.Formats[0], &formatprocessor.UnitH265{
						PTS: pts,
						AU:  unit.([][]byte),
						NTP: time.Now(),
					})
					if err != nil {
						s.Log(logger.Warn, "%v", err)
					}
				})

			case *codecs.MPEG4Audio:
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

				c.OnData(track, func(pts time.Duration, unit interface{}) {
					err := stream.writeData(medi, medi.Formats[0], &formatprocessor.UnitMPEG4Audio{
						PTS: pts,
						AUs: [][]byte{unit.([]byte)},
						NTP: time.Now(),
					})
					if err != nil {
						s.Log(logger.Warn, "%v", err)
					}
				})

			case *codecs.Opus:
				medi = &media.Media{
					Type: media.TypeAudio,
					Formats: []formats.Format{&formats.Opus{
						PayloadTyp: 96,
						IsStereo:   (tcodec.Channels == 2),
					}},
				}

				c.OnData(track, func(pts time.Duration, unit interface{}) {
					err := stream.writeData(medi, medi.Formats[0], &formatprocessor.UnitOpus{
						PTS:   pts,
						Frame: unit.([]byte),
						NTP:   time.Now(),
					})
					if err != nil {
						s.Log(logger.Warn, "%v", err)
					}
				})
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

		s.Log(logger.Info, "ready: %s", sourceMediaInfo(medias))
		stream = res.stream

		return nil
	})

	err := c.Start()
	if err != nil {
		return err
	}

	for {
		select {
		case err := <-c.Wait():
			c.Close()
			return err

		case <-reloadConf:

		case <-ctx.Done():
			c.Close()
			<-c.Wait()
			return nil
		}
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*hlsSource) apiSourceDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"hlsSource"}
}

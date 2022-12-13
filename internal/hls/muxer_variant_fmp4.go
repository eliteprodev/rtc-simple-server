package hls

import (
	"bytes"
	"net/http"
	"sync"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"

	"github.com/aler9/rtsp-simple-server/internal/hls/fmp4"
)

type muxerVariantFMP4 struct {
	playlist   *muxerVariantFMP4Playlist
	segmenter  *muxerVariantFMP4Segmenter
	videoTrack *format.H264
	audioTrack *format.MPEG4Audio

	mutex        sync.Mutex
	videoLastSPS []byte
	videoLastPPS []byte
	initContent  []byte
}

func newMuxerVariantFMP4(
	lowLatency bool,
	segmentCount int,
	segmentDuration time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack *format.H264,
	audioTrack *format.MPEG4Audio,
) *muxerVariantFMP4 {
	v := &muxerVariantFMP4{
		videoTrack: videoTrack,
		audioTrack: audioTrack,
	}

	v.playlist = newMuxerVariantFMP4Playlist(
		lowLatency,
		segmentCount,
		videoTrack,
		audioTrack,
	)

	v.segmenter = newMuxerVariantFMP4Segmenter(
		lowLatency,
		segmentCount,
		segmentDuration,
		partDuration,
		segmentMaxSize,
		videoTrack,
		audioTrack,
		v.playlist.onSegmentFinalized,
		v.playlist.onPartFinalized,
	)

	return v
}

func (v *muxerVariantFMP4) close() {
	v.playlist.close()
}

func (v *muxerVariantFMP4) writeH264(ntp time.Time, pts time.Duration, nalus [][]byte) error {
	return v.segmenter.writeH264(ntp, pts, nalus)
}

func (v *muxerVariantFMP4) writeAAC(ntp time.Time, pts time.Duration, au []byte) error {
	return v.segmenter.writeAAC(ntp, pts, au)
}

func (v *muxerVariantFMP4) file(name string, msn string, part string, skip string) *MuxerFileResponse {
	if name == "init.mp4" {
		v.mutex.Lock()
		defer v.mutex.Unlock()

		var sps []byte
		var pps []byte
		if v.videoTrack != nil {
			sps = v.videoTrack.SafeSPS()
			pps = v.videoTrack.SafePPS()
		}

		if v.initContent == nil ||
			(v.videoTrack != nil && (!bytes.Equal(v.videoLastSPS, sps) || !bytes.Equal(v.videoLastPPS, pps))) {
			init := fmp4.Init{}
			trackID := 1

			if v.videoTrack != nil {
				init.Tracks = append(init.Tracks, &fmp4.InitTrack{
					ID:        trackID,
					TimeScale: 90000,
					Format:    v.videoTrack,
				})
				trackID++
			}

			if v.audioTrack != nil {
				init.Tracks = append(init.Tracks, &fmp4.InitTrack{
					ID:        trackID,
					TimeScale: uint32(v.audioTrack.ClockRate()),
					Format:    v.audioTrack,
				})
			}

			initContent, err := init.Marshal()
			if err != nil {
				return &MuxerFileResponse{Status: http.StatusInternalServerError}
			}

			v.videoLastSPS = sps
			v.videoLastPPS = pps
			v.initContent = initContent
		}

		return &MuxerFileResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": "video/mp4",
			},
			Body: bytes.NewReader(v.initContent),
		}
	}

	return v.playlist.file(name, msn, part, skip)
}

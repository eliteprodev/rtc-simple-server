package hls

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/asticode/go-astits"
)

const (
	// an offset between PCR and PTS/DTS is needed to avoid PCR > PTS
	pcrOffset = 500 * time.Millisecond
)

type muxerTSSegment struct {
	hlsSegmentMaxSize uint64
	videoTrack        *gortsplib.TrackH264
	writeData         func(*astits.MuxerData) (int, error)

	startTime      time.Time
	name           string
	buf            bytes.Buffer
	startPTS       *time.Duration
	endPTS         time.Duration
	pcrSendCounter int
	audioAUCount   int
}

func newMuxerTSSegment(
	now time.Time,
	hlsSegmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	writeData func(*astits.MuxerData) (int, error),
) *muxerTSSegment {
	t := &muxerTSSegment{
		hlsSegmentMaxSize: hlsSegmentMaxSize,
		videoTrack:        videoTrack,
		writeData:         writeData,
		startTime:         now,
		name:              strconv.FormatInt(now.Unix(), 10),
	}

	// WriteTable() is called automatically when WriteData() is called with
	// - PID == PCRPID
	// - AdaptationField != nil
	// - RandomAccessIndicator = true

	return t
}

func (t *muxerTSSegment) duration() time.Duration {
	return t.endPTS - *t.startPTS
}

func (t *muxerTSSegment) write(p []byte) (int, error) {
	if uint64(len(p)+t.buf.Len()) > t.hlsSegmentMaxSize {
		return 0, fmt.Errorf("reached maximum segment size")
	}

	return t.buf.Write(p)
}

func (t *muxerTSSegment) reader() io.Reader {
	return bytes.NewReader(t.buf.Bytes())
}

func (t *muxerTSSegment) writeH264(
	pcr time.Duration,
	dts time.Duration,
	pts time.Duration,
	idrPresent bool,
	enc []byte,
) error {
	var af *astits.PacketAdaptationField

	if idrPresent {
		af = &astits.PacketAdaptationField{}
		af.RandomAccessIndicator = true
	}

	// send PCR once in a while
	if t.pcrSendCounter == 0 {
		if af == nil {
			af = &astits.PacketAdaptationField{}
		}
		af.HasPCR = true
		af.PCR = &astits.ClockReference{Base: int64(pcr.Seconds() * 90000)}
		t.pcrSendCounter = 3
	}
	t.pcrSendCounter--

	oh := &astits.PESOptionalHeader{
		MarkerBits: 2,
	}

	if dts == pts {
		oh.PTSDTSIndicator = astits.PTSDTSIndicatorOnlyPTS
		oh.PTS = &astits.ClockReference{Base: int64((pts + pcrOffset).Seconds() * 90000)}
	} else {
		oh.PTSDTSIndicator = astits.PTSDTSIndicatorBothPresent
		oh.DTS = &astits.ClockReference{Base: int64((dts + pcrOffset).Seconds() * 90000)}
		oh.PTS = &astits.ClockReference{Base: int64((pts + pcrOffset).Seconds() * 90000)}
	}

	_, err := t.writeData(&astits.MuxerData{
		PID:             256,
		AdaptationField: af,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: oh,
				StreamID:       224, // video
			},
			Data: enc,
		},
	})
	if err != nil {
		return err
	}

	if t.startPTS == nil {
		t.startPTS = &pts
	}

	if pts > t.endPTS {
		t.endPTS = pts
	}

	return nil
}

func (t *muxerTSSegment) writeAAC(
	pcr time.Duration,
	pts time.Duration,
	enc []byte,
	ausLen int,
) error {
	af := &astits.PacketAdaptationField{
		RandomAccessIndicator: true,
	}

	if t.videoTrack == nil {
		// send PCR once in a while
		if t.pcrSendCounter == 0 {
			af.HasPCR = true
			af.PCR = &astits.ClockReference{Base: int64(pcr.Seconds() * 90000)}
			t.pcrSendCounter = 3
		}
		t.pcrSendCounter--
	}

	_, err := t.writeData(&astits.MuxerData{
		PID:             257,
		AdaptationField: af,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: &astits.PESOptionalHeader{
					MarkerBits:      2,
					PTSDTSIndicator: astits.PTSDTSIndicatorOnlyPTS,
					PTS:             &astits.ClockReference{Base: int64((pts + pcrOffset).Seconds() * 90000)},
				},
				PacketLength: uint16(len(enc) + 8),
				StreamID:     192, // audio
			},
			Data: enc,
		},
	})
	if err != nil {
		return err
	}

	if t.videoTrack == nil {
		t.audioAUCount += ausLen
	}

	if t.startPTS == nil {
		t.startPTS = &pts
	}

	if pts > t.endPTS {
		t.endPTS = pts
	}

	return nil
}

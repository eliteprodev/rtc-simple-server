package formatprocessor

import (
	"bytes"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtph264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/pion/rtp"
)

// extract SPS and PPS without decoding RTP packets
func rtpH264ExtractSPSPPS(pkt *rtp.Packet) ([]byte, []byte) {
	if len(pkt.Payload) < 1 {
		return nil, nil
	}

	typ := h264.NALUType(pkt.Payload[0] & 0x1F)

	switch typ {
	case h264.NALUTypeSPS:
		return pkt.Payload, nil

	case h264.NALUTypePPS:
		return nil, pkt.Payload

	case h264.NALUTypeSTAPA:
		payload := pkt.Payload[1:]
		var sps []byte
		var pps []byte

		for len(payload) > 0 {
			if len(payload) < 2 {
				break
			}

			size := uint16(payload[0])<<8 | uint16(payload[1])
			payload = payload[2:]

			if size == 0 {
				break
			}

			if int(size) > len(payload) {
				return nil, nil
			}

			nalu := payload[:size]
			payload = payload[size:]

			typ = h264.NALUType(nalu[0] & 0x1F)

			switch typ {
			case h264.NALUTypeSPS:
				sps = nalu

			case h264.NALUTypePPS:
				pps = nalu
			}
		}

		return sps, pps

	default:
		return nil, nil
	}
}

// UnitH264 is a H264 data unit.
type UnitH264 struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        time.Duration
	AU         [][]byte
}

// GetRTPPackets implements Unit.
func (d *UnitH264) GetRTPPackets() []*rtp.Packet {
	return d.RTPPackets
}

// GetNTP implements Unit.
func (d *UnitH264) GetNTP() time.Time {
	return d.NTP
}

type formatProcessorH264 struct {
	udpMaxPayloadSize int
	format            *formats.H264

	encoder *rtph264.Encoder
	decoder *rtph264.Decoder
}

func newH264(
	udpMaxPayloadSize int,
	forma *formats.H264,
	allocateEncoder bool,
) (*formatProcessorH264, error) {
	t := &formatProcessorH264{
		udpMaxPayloadSize: udpMaxPayloadSize,
		format:            forma,
	}

	if allocateEncoder {
		t.encoder = forma.CreateEncoder()
	}

	return t, nil
}

func (t *formatProcessorH264) updateTrackParametersFromRTPPacket(pkt *rtp.Packet) {
	sps, pps := rtpH264ExtractSPSPPS(pkt)
	update := false

	if sps != nil && !bytes.Equal(sps, t.format.SPS) {
		update = true
	}

	if pps != nil && !bytes.Equal(pps, t.format.PPS) {
		update = true
	}

	if update {
		if sps == nil {
			sps = t.format.SPS
		}
		if pps == nil {
			pps = t.format.PPS
		}
		t.format.SafeSetParams(sps, pps)
	}
}

func (t *formatProcessorH264) updateTrackParametersFromNALUs(nalus [][]byte) {
	sps := t.format.SPS
	pps := t.format.PPS
	update := false

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS:
			if !bytes.Equal(nalu, sps) {
				sps = nalu
				update = true
			}

		case h264.NALUTypePPS:
			if !bytes.Equal(nalu, pps) {
				pps = nalu
				update = true
			}
		}
	}

	if update {
		t.format.SafeSetParams(sps, pps)
	}
}

func (t *formatProcessorH264) remuxAccessUnit(nalus [][]byte) [][]byte {
	addParameters := false
	n := 0

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS: // remove parameters
			continue

		case h264.NALUTypeAccessUnitDelimiter: // remove AUDs
			continue

		case h264.NALUTypeIDR: // prepend parameters if there's at least an IDR
			if !addParameters {
				addParameters = true

				if t.format.SPS != nil && t.format.PPS != nil {
					n += 2
				}
			}
		}
		n++
	}

	if n == 0 {
		return nil
	}

	filteredNALUs := make([][]byte, n)
	i := 0

	if addParameters && t.format.SPS != nil && t.format.PPS != nil {
		filteredNALUs[0] = t.format.SPS
		filteredNALUs[1] = t.format.PPS
		i = 2
	}

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			continue

		case h264.NALUTypeAccessUnitDelimiter:
			continue
		}

		filteredNALUs[i] = nalu
		i++
	}

	return filteredNALUs
}

func (t *formatProcessorH264) Process(unit Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := unit.(*UnitH264)

	if tunit.RTPPackets != nil {
		pkt := tunit.RTPPackets[0]
		t.updateTrackParametersFromRTPPacket(pkt)

		if t.encoder == nil {
			// remove padding
			pkt.Header.Padding = false
			pkt.PaddingSize = 0

			// RTP packets exceed maximum size: start re-encoding them
			if pkt.MarshalSize() > t.udpMaxPayloadSize {
				v1 := pkt.SSRC
				v2 := pkt.SequenceNumber
				v3 := pkt.Timestamp
				t.encoder = &rtph264.Encoder{
					PayloadMaxSize:        t.udpMaxPayloadSize - 12,
					PayloadType:           pkt.PayloadType,
					SSRC:                  &v1,
					InitialSequenceNumber: &v2,
					InitialTimestamp:      &v3,
					PacketizationMode:     t.format.PacketizationMode,
				}
				t.encoder.Init()
			}
		}

		// decode from RTP
		if hasNonRTSPReaders || t.encoder != nil {
			if t.decoder == nil {
				t.decoder = t.format.CreateDecoder()
			}

			if t.encoder != nil {
				tunit.RTPPackets = nil
			}

			// DecodeUntilMarker() is necessary, otherwise Encode() generates partial groups
			au, pts, err := t.decoder.DecodeUntilMarker(pkt)
			if err != nil {
				if err == rtph264.ErrNonStartingPacketAndNoPrevious || err == rtph264.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tunit.AU = au
			tunit.PTS = pts
			tunit.AU = t.remuxAccessUnit(tunit.AU)
		}

		// route packet as is
		if t.encoder == nil {
			return nil
		}
	} else {
		t.updateTrackParametersFromNALUs(tunit.AU)
		tunit.AU = t.remuxAccessUnit(tunit.AU)
	}

	if len(tunit.AU) != 0 {
		pkts, err := t.encoder.Encode(tunit.AU, tunit.PTS)
		if err != nil {
			return err
		}
		tunit.RTPPackets = pkts
	} else {
		tunit.RTPPackets = nil
	}

	return nil
}

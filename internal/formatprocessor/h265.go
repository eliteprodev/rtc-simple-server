package formatprocessor

import (
	"bytes"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtph265"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/pion/rtp"
)

// extract VPS, SPS and PPS without decoding RTP packets
func rtpH265ExtractVPSSPSPPS(pkt *rtp.Packet) ([]byte, []byte, []byte) {
	if len(pkt.Payload) < 2 {
		return nil, nil, nil
	}

	typ := h265.NALUType((pkt.Payload[0] >> 1) & 0b111111)

	switch typ {
	case h265.NALUType_VPS_NUT:
		return pkt.Payload, nil, nil

	case h265.NALUType_SPS_NUT:
		return nil, pkt.Payload, nil

	case h265.NALUType_PPS_NUT:
		return nil, nil, pkt.Payload

	case h265.NALUType_AggregationUnit:
		payload := pkt.Payload[2:]
		var vps []byte
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
				return nil, nil, nil
			}

			nalu := payload[:size]
			payload = payload[size:]

			typ = h265.NALUType((pkt.Payload[0] >> 1) & 0b111111)

			switch typ {
			case h265.NALUType_VPS_NUT:
				vps = nalu

			case h265.NALUType_SPS_NUT:
				sps = nalu

			case h265.NALUType_PPS_NUT:
				pps = nalu
			}
		}

		return vps, sps, pps

	default:
		return nil, nil, nil
	}
}

// UnitH265 is a H265 data unit.
type UnitH265 struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        time.Duration
	AU         [][]byte
}

// GetRTPPackets implements Unit.
func (d *UnitH265) GetRTPPackets() []*rtp.Packet {
	return d.RTPPackets
}

// GetNTP implements Unit.
func (d *UnitH265) GetNTP() time.Time {
	return d.NTP
}

type formatProcessorH265 struct {
	udpMaxPayloadSize int
	format            *formats.H265

	encoder *rtph265.Encoder
	decoder *rtph265.Decoder
}

func newH265(
	udpMaxPayloadSize int,
	forma *formats.H265,
	allocateEncoder bool,
) (*formatProcessorH265, error) {
	t := &formatProcessorH265{
		udpMaxPayloadSize: udpMaxPayloadSize,
		format:            forma,
	}

	if allocateEncoder {
		t.encoder = forma.CreateEncoder()
	}

	return t, nil
}

func (t *formatProcessorH265) updateTrackParametersFromRTPPacket(pkt *rtp.Packet) {
	vps, sps, pps := rtpH265ExtractVPSSPSPPS(pkt)
	update := false

	if vps != nil && !bytes.Equal(vps, t.format.VPS) {
		update = true
	}

	if sps != nil && !bytes.Equal(sps, t.format.SPS) {
		update = true
	}

	if pps != nil && !bytes.Equal(pps, t.format.PPS) {
		update = true
	}

	if update {
		if vps == nil {
			vps = t.format.VPS
		}
		if sps == nil {
			sps = t.format.SPS
		}
		if pps == nil {
			pps = t.format.PPS
		}
		t.format.SafeSetParams(vps, sps, pps)
	}
}

func (t *formatProcessorH265) updateTrackParametersFromNALUs(nalus [][]byte) {
	vps := t.format.VPS
	sps := t.format.SPS
	pps := t.format.PPS
	update := false

	for _, nalu := range nalus {
		typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case h265.NALUType_VPS_NUT:
			if !bytes.Equal(nalu, t.format.VPS) {
				vps = nalu
				update = true
			}

		case h265.NALUType_SPS_NUT:
			if !bytes.Equal(nalu, t.format.SPS) {
				sps = nalu
				update = true
			}

		case h265.NALUType_PPS_NUT:
			if !bytes.Equal(nalu, t.format.PPS) {
				pps = nalu
				update = true
			}
		}
	}

	if update {
		t.format.SafeSetParams(vps, sps, pps)
	}
}

func (t *formatProcessorH265) remuxAccessUnit(nalus [][]byte) [][]byte {
	addParameters := false
	n := 0

	for _, nalu := range nalus {
		typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT: // remove parameters
			continue

		case h265.NALUType_AUD_NUT: // remove AUDs
			continue

		// prepend parameters if there's at least a random access unit
		case h265.NALUType_IDR_W_RADL, h265.NALUType_IDR_N_LP, h265.NALUType_CRA_NUT:
			if !addParameters {
				addParameters = true

				if t.format.VPS != nil && t.format.SPS != nil && t.format.PPS != nil {
					n += 3
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

	if addParameters && t.format.VPS != nil && t.format.SPS != nil && t.format.PPS != nil {
		filteredNALUs[0] = t.format.VPS
		filteredNALUs[1] = t.format.SPS
		filteredNALUs[2] = t.format.PPS
		i = 3
	}

	for _, nalu := range nalus {
		typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT:
			continue

		case h265.NALUType_AUD_NUT:
			continue
		}

		filteredNALUs[i] = nalu
		i++
	}

	return filteredNALUs
}

func (t *formatProcessorH265) Process(unit Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := unit.(*UnitH265)

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
				t.encoder = &rtph265.Encoder{
					PayloadMaxSize:        t.udpMaxPayloadSize - 12,
					PayloadType:           pkt.PayloadType,
					SSRC:                  &v1,
					InitialSequenceNumber: &v2,
					InitialTimestamp:      &v3,
					MaxDONDiff:            t.format.MaxDONDiff,
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
				if err == rtph265.ErrNonStartingPacketAndNoPrevious || err == rtph265.ErrMorePacketsNeeded {
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

package rtmp

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
	"github.com/notedit/rtmp/av"
	nh264 "github.com/notedit/rtmp/codec/h264"
	"github.com/notedit/rtmp/format/flv/flvio"
	"github.com/notedit/rtmp/format/rtmp"
)

const (
	readBufferSize  = 4096
	writeBufferSize = 4096
	codecH264       = 7
	codecAAC        = 10
)

// Conn is a RTMP connection.
type Conn struct {
	rconn *rtmp.Conn
	nconn net.Conn
}

// Close closes the connection.
func (c *Conn) Close() error {
	return c.nconn.Close()
}

// ClientHandshake performs the handshake of a client-side connection.
func (c *Conn) ClientHandshake() error {
	return c.rconn.Prepare(rtmp.StageGotPublishOrPlayCommand, rtmp.PrepareReading)
}

// ServerHandshake performs the handshake of a server-side connection.
func (c *Conn) ServerHandshake() error {
	return c.rconn.Prepare(rtmp.StageGotPublishOrPlayCommand, 0)
}

// SetReadDeadline sets the read deadline.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.nconn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.nconn.SetWriteDeadline(t)
}

// RemoteAddr returns the remote network address.
func (c *Conn) RemoteAddr() net.Addr {
	return c.nconn.RemoteAddr()
}

// IsPublishing returns whether the connection is publishing.
func (c *Conn) IsPublishing() bool {
	return c.rconn.Publishing
}

// URL returns the URL requested by the connection.
func (c *Conn) URL() *url.URL {
	return c.rconn.URL
}

// ReadPacket reads a packet.
func (c *Conn) ReadPacket() (av.Packet, error) {
	return c.rconn.ReadPacket()
}

// WritePacket writes a packet.
func (c *Conn) WritePacket(pkt av.Packet) error {
	err := c.rconn.WritePacket(pkt)
	if err != nil {
		return err
	}
	return c.rconn.FlushWrite()
}

// ReadMetadata reads track informations.
func (c *Conn) ReadMetadata() (*gortsplib.TrackH264, *gortsplib.TrackAAC, error) {
	var videoTrack *gortsplib.TrackH264
	var audioTrack *gortsplib.TrackAAC

	md, err := func() (flvio.AMFMap, error) {
		pkt, err := c.ReadPacket()
		if err != nil {
			return nil, err
		}

		if pkt.Type != av.Metadata {
			return nil, fmt.Errorf("first packet must be metadata")
		}

		arr, err := flvio.ParseAMFVals(pkt.Data, false)
		if err != nil {
			return nil, err
		}

		if len(arr) != 1 {
			return nil, fmt.Errorf("invalid metadata")
		}

		ma, ok := arr[0].(flvio.AMFMap)
		if !ok {
			return nil, fmt.Errorf("invalid metadata")
		}

		return ma, nil
	}()
	if err != nil {
		return nil, nil, err
	}

	hasVideo, err := func() (bool, error) {
		v, ok := md.GetV("videocodecid")
		if !ok {
			return false, nil
		}

		switch vt := v.(type) {
		case float64:
			switch vt {
			case 0:
				return false, nil

			case codecH264:
				return true, nil
			}

		case string:
			if vt == "avc1" {
				return true, nil
			}
		}

		return false, fmt.Errorf("unsupported video codec %v", v)
	}()
	if err != nil {
		return nil, nil, err
	}

	hasAudio, err := func() (bool, error) {
		v, ok := md.GetV("audiocodecid")
		if !ok {
			return false, nil
		}

		switch vt := v.(type) {
		case float64:
			switch vt {
			case 0:
				return false, nil

			case codecAAC:
				return true, nil
			}

		case string:
			if vt == "mp4a" {
				return true, nil
			}
		}

		return false, fmt.Errorf("unsupported audio codec %v", v)
	}()
	if err != nil {
		return nil, nil, err
	}

	if !hasVideo && !hasAudio {
		return nil, nil, fmt.Errorf("stream doesn't contain tracks with supported codecs (H264 or AAC)")
	}

	for {
		var pkt av.Packet
		pkt, err = c.ReadPacket()
		if err != nil {
			return nil, nil, err
		}

		switch pkt.Type {
		case av.H264DecoderConfig:
			if !hasVideo {
				return nil, nil, fmt.Errorf("unexpected video packet")
			}

			if videoTrack != nil {
				return nil, nil, fmt.Errorf("video track setupped twice")
			}

			codec, err := nh264.FromDecoderConfig(pkt.Data)
			if err != nil {
				return nil, nil, err
			}

			videoTrack, err = gortsplib.NewTrackH264(96, codec.SPS[0], codec.PPS[0], nil)
			if err != nil {
				return nil, nil, err
			}

		case av.AACDecoderConfig:
			if !hasAudio {
				return nil, nil, fmt.Errorf("unexpected audio packet")
			}

			if audioTrack != nil {
				return nil, nil, fmt.Errorf("audio track setupped twice")
			}

			var mpegConf aac.MPEG4AudioConfig
			err := mpegConf.Decode(pkt.Data)
			if err != nil {
				return nil, nil, err
			}

			audioTrack, err = gortsplib.NewTrackAAC(96, int(mpegConf.Type), mpegConf.SampleRate,
				mpegConf.ChannelCount, mpegConf.AOTSpecificConfig)
			if err != nil {
				return nil, nil, err
			}
		}

		if (!hasVideo || videoTrack != nil) &&
			(!hasAudio || audioTrack != nil) {
			return videoTrack, audioTrack, nil
		}
	}
}

// WriteMetadata writes track informations.
func (c *Conn) WriteMetadata(videoTrack *gortsplib.TrackH264, audioTrack *gortsplib.TrackAAC) error {
	err := c.WritePacket(av.Packet{
		Type: av.Metadata,
		Data: flvio.FillAMF0ValMalloc(flvio.AMFMap{
			{
				K: "videodatarate",
				V: float64(0),
			},
			{
				K: "videocodecid",
				V: func() float64 {
					if videoTrack != nil {
						return codecH264
					}
					return 0
				}(),
			},
			{
				K: "audiodatarate",
				V: float64(0),
			},
			{
				K: "audiocodecid",
				V: func() float64 {
					if audioTrack != nil {
						return codecAAC
					}
					return 0
				}(),
			},
		}),
	})
	if err != nil {
		return err
	}

	if videoTrack != nil {
		if videoTrack.SPS() == nil || videoTrack.PPS() == nil {
			return fmt.Errorf("invalid H264 track: SPS or PPS not provided into the SDP")
		}

		codec := nh264.Codec{
			SPS: map[int][]byte{
				0: videoTrack.SPS(),
			},
			PPS: map[int][]byte{
				0: videoTrack.PPS(),
			},
		}
		b := make([]byte, 128)
		var n int
		codec.ToConfig(b, &n)
		b = b[:n]

		err = c.WritePacket(av.Packet{
			Type: av.H264DecoderConfig,
			Data: b,
		})
		if err != nil {
			return err
		}
	}

	if audioTrack != nil {
		enc, err := aac.MPEG4AudioConfig{
			Type:              aac.MPEG4AudioType(audioTrack.Type()),
			SampleRate:        audioTrack.ClockRate(),
			ChannelCount:      audioTrack.ChannelCount(),
			AOTSpecificConfig: audioTrack.AOTSpecificConfig(),
		}.Encode()
		if err != nil {
			return err
		}

		err = c.WritePacket(av.Packet{
			Type: av.AACDecoderConfig,
			Data: enc,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

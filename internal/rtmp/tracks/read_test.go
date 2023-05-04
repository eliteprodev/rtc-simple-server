package tracks

import (
	"bytes"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/notedit/rtmp/format/flv/flvio"
	"github.com/stretchr/testify/require"

	"github.com/aler9/mediamtx/internal/rtmp/bytecounter"
	"github.com/aler9/mediamtx/internal/rtmp/h264conf"
	"github.com/aler9/mediamtx/internal/rtmp/message"
)

func TestRead(t *testing.T) {
	sps := []byte{
		0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
		0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
		0x00, 0x03, 0x00, 0x3d, 0x08,
	}

	pps := []byte{
		0x68, 0xee, 0x3c, 0x80,
	}

	for _, ca := range []struct {
		name       string
		videoTrack formats.Format
		audioTrack formats.Format
	}{
		{
			"video+audio",
			&formats.H264{
				PayloadTyp:        96,
				SPS:               sps,
				PPS:               pps,
				PacketizationMode: 1,
			},
			&formats.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
		},
		{
			"video",
			&formats.H264{
				PayloadTyp:        96,
				SPS:               sps,
				PPS:               pps,
				PacketizationMode: 1,
			},
			nil,
		},
		{
			"metadata without codec id",
			&formats.H264{
				PayloadTyp:        96,
				SPS:               sps,
				PPS:               pps,
				PacketizationMode: 1,
			},
			&formats.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
		},
		{
			"missing metadata, video+audio",
			&formats.H264{
				PayloadTyp:        96,
				SPS:               sps,
				PPS:               pps,
				PacketizationMode: 1,
			},
			&formats.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
		},
		{
			"missing metadata, audio",
			nil,
			&formats.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
		},
		{
			"obs studio pre 29.1 h265",
			&formats.H265{
				PayloadTyp: 96,
				VPS: []byte{
					0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x01, 0x40,
					0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00,
					0x03, 0x00, 0x00, 0x03, 0x00, 0x7b, 0xac, 0x09,
				},
				SPS: []byte{
					0x42, 0x01, 0x01, 0x01, 0x40, 0x00, 0x00, 0x03,
					0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00,
					0x03, 0x00, 0x7b, 0xa0, 0x03, 0xc0, 0x80, 0x11,
					0x07, 0xcb, 0x96, 0xb4, 0xa4, 0x25, 0x92, 0xe3,
					0x01, 0x6a, 0x02, 0x02, 0x02, 0x08, 0x00, 0x00,
					0x03, 0x00, 0x08, 0x00, 0x00, 0x03, 0x01, 0xe3,
					0x00, 0x2e, 0xf2, 0x88, 0x00, 0x09, 0x89, 0x60,
					0x00, 0x04, 0xc4, 0xb4, 0x20,
				},
				PPS: []byte{
					0x44, 0x01, 0xc0, 0xf7, 0xc0, 0xcc, 0x90,
				},
			},
			&formats.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			var buf bytes.Buffer
			bc := bytecounter.NewReadWriter(&buf)
			mrw := message.NewReadWriter(bc, true)

			switch ca.name {
			case "video+audio":
				err := mrw.Write(&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						flvio.AMFMap{
							{
								K: "videodatarate",
								V: float64(0),
							},
							{
								K: "videocodecid",
								V: float64(message.CodecH264),
							},
							{
								K: "audiodatarate",
								V: float64(0),
							},
							{
								K: "audiocodecid",
								V: float64(message.CodecMPEG4Audio),
							},
						},
					},
				})
				require.NoError(t, err)

				buf, _ := h264conf.Conf{
					SPS: sps,
					PPS: pps,
				}.Marshal()

				err = mrw.Write(&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload:         buf,
				})
				require.NoError(t, err)

				enc, err := mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)

				err = mrw.Write(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         message.AudioAACTypeConfig,
					Payload:         enc,
				})
				require.NoError(t, err)

			case "video":
				err := mrw.Write(&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						flvio.AMFMap{
							{
								K: "videodatarate",
								V: float64(0),
							},
							{
								K: "videocodecid",
								V: float64(message.CodecH264),
							},
							{
								K: "audiodatarate",
								V: float64(0),
							},
							{
								K: "audiocodecid",
								V: float64(0),
							},
						},
					},
				})
				require.NoError(t, err)

				buf, _ := h264conf.Conf{
					SPS: sps,
					PPS: pps,
				}.Marshal()

				err = mrw.Write(&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload:         buf,
				})
				require.NoError(t, err)

			case "metadata without codec id":
				err := mrw.Write(&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						flvio.AMFMap{
							{
								K: "width",
								V: float64(2688),
							},
							{
								K: "height",
								V: float64(1520),
							},
							{
								K: "framerate",
								V: float64(0o25),
							},
						},
					},
				})
				require.NoError(t, err)

				buf, _ := h264conf.Conf{
					SPS: sps,
					PPS: pps,
				}.Marshal()

				err = mrw.Write(&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload:         buf,
				})
				require.NoError(t, err)

				enc, err := mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)

				err = mrw.Write(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         message.AudioAACTypeConfig,
					Payload:         enc,
				})
				require.NoError(t, err)

			case "missing metadata, video+audio":
				buf, _ := h264conf.Conf{
					SPS: sps,
					PPS: pps,
				}.Marshal()

				err := mrw.Write(&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload:         buf,
				})
				require.NoError(t, err)

				enc, err := mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)

				err = mrw.Write(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         message.AudioAACTypeConfig,
					Payload:         enc,
				})
				require.NoError(t, err)

			case "missing metadata, audio":
				enc, err := mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)

				err = mrw.Write(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         message.AudioAACTypeConfig,
					Payload:         enc,
				})
				require.NoError(t, err)

				err = mrw.Write(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         message.AudioAACTypeConfig,
					Payload:         enc,
					DTS:             1 * time.Second,
				})
				require.NoError(t, err)

			case "obs studio pre 29.1 h265":
				err := mrw.Write(&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						flvio.AMFMap{
							{
								K: "videodatarate",
								V: float64(0),
							},
							{
								K: "videocodecid",
								V: float64(message.CodecH264),
							},
							{
								K: "audiodatarate",
								V: float64(0),
							},
							{
								K: "audiocodecid",
								V: float64(message.CodecMPEG4Audio),
							},
						},
					},
				})
				require.NoError(t, err)

				avcc, err := h264.AVCCMarshal([][]byte{
					{ // VPS
						0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x01, 0x40,
						0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00,
						0x03, 0x00, 0x00, 0x03, 0x00, 0x7b, 0xac, 0x09,
					},
					{ // SPS
						0x42, 0x01, 0x01, 0x01, 0x40, 0x00, 0x00, 0x03,
						0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00,
						0x03, 0x00, 0x7b, 0xa0, 0x03, 0xc0, 0x80, 0x11,
						0x07, 0xcb, 0x96, 0xb4, 0xa4, 0x25, 0x92, 0xe3,
						0x01, 0x6a, 0x02, 0x02, 0x02, 0x08, 0x00, 0x00,
						0x03, 0x00, 0x08, 0x00, 0x00, 0x03, 0x01, 0xe3,
						0x00, 0x2e, 0xf2, 0x88, 0x00, 0x09, 0x89, 0x60,
						0x00, 0x04, 0xc4, 0xb4, 0x20,
					},
					{
						// PPS
						0x44, 0x01, 0xc0, 0xf7, 0xc0, 0xcc, 0x90,
					},
				})
				require.NoError(t, err)

				err = mrw.Write(&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeAU,
					Payload:         avcc,
				})
				require.NoError(t, err)

				enc, err := mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)

				err = mrw.Write(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         message.AudioAACTypeConfig,
					Payload:         enc,
				})
				require.NoError(t, err)
			}

			videoTrack, audioTrack, err := Read(mrw)
			require.NoError(t, err)
			require.Equal(t, ca.videoTrack, videoTrack)
			require.Equal(t, ca.audioTrack, audioTrack)
		})
	}
}

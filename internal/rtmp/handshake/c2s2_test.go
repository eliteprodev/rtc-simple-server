package handshake

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestC2S2Read(t *testing.T) {
	c2s2dec := C2S2{
		Time:  435234723,
		Time2: 7893542,
		Random: append(
			bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 372),
			[]byte{
				0x01, 0x02, 0x03, 0x04, 0x01, 0x02, 0x03, 0x04,
				0x96, 0x07, 0x2f, 0xe4, 0x04, 0xc5, 0x84, 0xa2,
				0x21, 0x05, 0xcc, 0xb5, 0x7f, 0x93, 0x02, 0x14,
				0xaf, 0xb0, 0x76, 0x75, 0xfd, 0x82, 0x29, 0xbe,
				0xb9, 0x27, 0x9d, 0x4b, 0x0c, 0x81, 0x13, 0xec,
			}...),
		Digest: []byte{
			0x3f, 0xd0, 0xb1, 0xdf, 0xed, 0x6c, 0x9b, 0xc3,
			0x73, 0x68, 0xe2, 0x47, 0x92, 0x59, 0x32, 0x9a,
			0x3a, 0xc9, 0x1e, 0xeb, 0xfc, 0xad, 0x8e, 0x9d,
			0x4e, 0xf4, 0x30, 0xb1, 0x9a, 0xc9, 0x15, 0x99,
		},
	}

	c2s2enc := append(append(
		[]byte{
			0x19, 0xf1, 0x27, 0xa3, 0x00, 0x78, 0x72, 0x26,
		},
		bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 374)...,
	), []byte{
		0x96, 0x07, 0x2f, 0xe4, 0x04, 0xc5, 0x84, 0xa2,
		0x21, 0x05, 0xcc, 0xb5, 0x7f, 0x93, 0x02, 0x14,
		0xaf, 0xb0, 0x76, 0x75, 0xfd, 0x82, 0x29, 0xbe,
		0xb9, 0x27, 0x9d, 0x4b, 0x0c, 0x81, 0x13, 0xec,
	}...)

	var c2s2 C2S2
	c2s2.Digest = c2s2dec.Digest
	err := c2s2.Read(bytes.NewReader(c2s2enc))
	require.NoError(t, err)
	require.Equal(t, c2s2dec, c2s2)
}

func TestC2S2Write(t *testing.T) {
	c2s2dec := C2S2{
		Time:   435234723,
		Time2:  7893542,
		Random: bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 382),
		Digest: []byte{
			0x3f, 0xd0, 0xb1, 0xdf, 0xed, 0x6c, 0x9b, 0xc3,
			0x73, 0x68, 0xe2, 0x47, 0x92, 0x59, 0x32, 0x9a,
			0x3a, 0xc9, 0x1e, 0xeb, 0xfc, 0xad, 0x8e, 0x9d,
			0x4e, 0xf4, 0x30, 0xb1, 0x9a, 0xc9, 0x15, 0x99,
		},
	}

	c2s2enc := append(append(
		[]byte{
			0x19, 0xf1, 0x27, 0xa3, 0x00, 0x78, 0x72, 0x26,
		},
		bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 374)...,
	), []byte{
		0x96, 0x07, 0x2f, 0xe4, 0x04, 0xc5, 0x84, 0xa2,
		0x21, 0x05, 0xcc, 0xb5, 0x7f, 0x93, 0x02, 0x14,
		0xaf, 0xb0, 0x76, 0x75, 0xfd, 0x82, 0x29, 0xbe,
		0xb9, 0x27, 0x9d, 0x4b, 0x0c, 0x81, 0x13, 0xec,
	}...)

	var buf bytes.Buffer
	err := c2s2dec.Write(&buf)
	require.NoError(t, err)
	require.Equal(t, c2s2enc, buf.Bytes())
}

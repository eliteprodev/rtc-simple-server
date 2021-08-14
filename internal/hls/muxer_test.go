package hls

import (
	"io/ioutil"
	"regexp"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/stretchr/testify/require"
)

func checkTSPacket(t *testing.T, byts []byte, pid int, afc int) {
	require.Equal(t, byte(0x47), byts[0])                                      // sync bit
	require.Equal(t, uint16(pid), (uint16(byts[1])<<8|uint16(byts[2]))&0x1fff) // PID
	require.Equal(t, uint8(afc), (byts[3]>>4)&0x03)                            // adaptation field control
}

func TestMuxer(t *testing.T) {
	videoTrack, err := gortsplib.NewTrackH264(96, []byte{0x07, 0x01, 0x02, 0x03}, []byte{0x08})
	require.NoError(t, err)

	audioTrack, err := gortsplib.NewTrackAAC(97, []byte{17, 144})
	require.NoError(t, err)

	m, err := NewMuxer(3, 1*time.Second, videoTrack, audioTrack)
	require.NoError(t, err)
	defer m.Close()

	// group without IDR
	err = m.WriteH264(1*time.Second, [][]byte{
		{0x06},
		{0x07},
	})
	require.NoError(t, err)

	// group with IDR
	err = m.WriteH264(2*time.Second, [][]byte{
		{5}, // IDR
		{9}, // AUD
		{8}, // PPS
		{7}, // SPS
	})
	require.NoError(t, err)

	err = m.WriteAAC(3*time.Second, [][]byte{
		{0x01, 0x02, 0x03, 0x04},
		{0x05, 0x06, 0x07, 0x08},
	})
	require.NoError(t, err)

	// group without IDR
	err = m.WriteH264(4*time.Second, [][]byte{
		{6},
		{7},
	})
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	// group with IDR
	err = m.WriteH264(6*time.Second, [][]byte{
		{5}, // IDR
	})
	require.NoError(t, err)

	byts, err := ioutil.ReadAll(m.Playlist())
	require.NoError(t, err)

	re := regexp.MustCompile(`^#EXTM3U\n` +
		`#EXT-X-VERSION:3\n` +
		`#EXT-X-ALLOW-CACHE:NO\n` +
		`#EXT-X-TARGETDURATION:2\n` +
		`#EXT-X-MEDIA-SEQUENCE:0\n` +
		`#EXTINF:2,\n` +
		`([0-9]+\.ts)\n` +
		`#EXTINF:0,\n` +
		`([0-9]+\.ts)\n$`)
	ma := re.FindStringSubmatch(string(byts))
	require.NotEqual(t, nil, ma)

	byts, err = ioutil.ReadAll(m.TSFile(ma[1]))
	require.NoError(t, err)

	checkTSPacket(t, byts, 0, 1)
	byts = byts[188:]
	checkTSPacket(t, byts, 4096, 1)
	byts = byts[188:]

	checkTSPacket(t, byts, 256, 3)
	alen := int(byts[4])
	byts = byts[4+alen+20:]
	require.Equal(t,
		[]byte{
			0, 0, 0, 1, 9, 240, // AUD
			0, 0, 0, 1, 7, 1, 2, 3, // SPS
			0, 0, 0, 1, 8, // PPS
			0, 0, 0, 1, 5, // IDR
		},
		byts[:24],
	)
}

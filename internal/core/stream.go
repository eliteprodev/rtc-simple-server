package core

import (
	"sync"

	"github.com/aler9/gortsplib"
)

type streamNonRTSPReadersMap struct {
	mutex sync.RWMutex
	ma    map[reader]struct{}
}

func newStreamNonRTSPReadersMap() *streamNonRTSPReadersMap {
	return &streamNonRTSPReadersMap{
		ma: make(map[reader]struct{}),
	}
}

func (m *streamNonRTSPReadersMap) close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.ma = nil
}

func (m *streamNonRTSPReadersMap) add(r reader) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.ma[r] = struct{}{}
}

func (m *streamNonRTSPReadersMap) remove(r reader) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.ma, r)
}

func (m *streamNonRTSPReadersMap) forwardPacketRTP(trackID int, payload []byte) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for c := range m.ma {
		c.onReaderPacketRTP(trackID, payload)
	}
}

func (m *streamNonRTSPReadersMap) forwardPacketRTCP(trackID int, payload []byte) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for c := range m.ma {
		c.onReaderPacketRTCP(trackID, payload)
	}
}

type stream struct {
	nonRTSPReaders *streamNonRTSPReadersMap
	rtspStream     *gortsplib.ServerStream
}

func newStream(tracks gortsplib.Tracks) *stream {
	s := &stream{
		nonRTSPReaders: newStreamNonRTSPReadersMap(),
		rtspStream:     gortsplib.NewServerStream(tracks),
	}
	return s
}

func (s *stream) close() {
	s.nonRTSPReaders.close()
	s.rtspStream.Close()
}

func (s *stream) tracks() gortsplib.Tracks {
	return s.rtspStream.Tracks()
}

func (s *stream) readerAdd(r reader) {
	if _, ok := r.(pathRTSPSession); !ok {
		s.nonRTSPReaders.add(r)
	}
}

func (s *stream) readerRemove(r reader) {
	if _, ok := r.(pathRTSPSession); !ok {
		s.nonRTSPReaders.remove(r)
	}
}

func (s *stream) onPacketRTP(trackID int, payload []byte) {
	// forward to RTSP readers
	s.rtspStream.WritePacketRTP(trackID, payload)

	// forward to non-RTSP readers
	s.nonRTSPReaders.forwardPacketRTP(trackID, payload)
}

func (s *stream) onPacketRTCP(trackID int, payload []byte) {
	// forward to RTSP readers
	s.rtspStream.WritePacketRTCP(trackID, payload)

	// forward to non-RTSP readers
	s.nonRTSPReaders.forwardPacketRTCP(trackID, payload)
}

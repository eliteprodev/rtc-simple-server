package message

import (
	"github.com/aler9/mediamtx/internal/rtmp/bytecounter"
)

// ReadWriter is a message reader/writer.
type ReadWriter struct {
	r *Reader
	w *Writer
}

// NewReadWriter allocates a ReadWriter.
func NewReadWriter(bc *bytecounter.ReadWriter, checkAcknowledge bool) *ReadWriter {
	w := NewWriter(bc.Writer, checkAcknowledge)

	r := NewReader(bc.Reader, func(count uint32) error {
		return w.Write(&Acknowledge{
			Value: count,
		})
	})

	return &ReadWriter{
		r: r,
		w: w,
	}
}

// Read reads a message.
func (rw *ReadWriter) Read() (Message, error) {
	msg, err := rw.r.Read()
	if err != nil {
		return nil, err
	}

	switch tmsg := msg.(type) {
	case *Acknowledge:
		rw.w.SetAcknowledgeValue(tmsg.Value)

	case *UserControlPingRequest:
		rw.w.Write(&UserControlPingResponse{
			ServerTime: tmsg.ServerTime,
		})
	}

	return msg, nil
}

// Write writes a message.
func (rw *ReadWriter) Write(msg Message) error {
	return rw.w.Write(msg)
}

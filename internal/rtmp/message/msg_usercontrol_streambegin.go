package message

import (
	"encoding/binary"
	"fmt"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// MsgUserControlStreamBegin is a user control message.
type MsgUserControlStreamBegin struct {
	StreamID uint32
}

// Unmarshal implements Message.
func (m *MsgUserControlStreamBegin) Unmarshal(raw *rawmessage.Message) error {
	if raw.ChunkStreamID != ControlChunkStreamID {
		return fmt.Errorf("unexpected chunk stream ID")
	}

	if len(raw.Body) != 6 {
		return fmt.Errorf("invalid body size")
	}

	m.StreamID = binary.BigEndian.Uint32(raw.Body[2:])

	return nil
}

// Marshal implements Message.
func (m MsgUserControlStreamBegin) Marshal() (*rawmessage.Message, error) {
	body := make([]byte, 6)
	binary.BigEndian.PutUint16(body, UserControlTypeStreamBegin)
	binary.BigEndian.PutUint32(body[2:], m.StreamID)

	return &rawmessage.Message{
		ChunkStreamID: ControlChunkStreamID,
		Type:          chunk.MessageTypeUserControl,
		Body:          body,
	}, nil
}

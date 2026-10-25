package peer

import (
	"encoding/binary"
	"fmt"
	"io"
)

type MessageID uint8

const (
	MsgChoke MessageID = iota
	MsgUnchoke
	MsgInterested
	MsgNotInterested
	MsgHave
	MsgBitfield
	MsgRequest
	MsgPiece
	MsgCancel
)

var msgNames = [...]string{"choke", "unchoke", "interested", "not interested", "have", "bitfield", "request", "piece", "cancel"}

func (id MessageID) String() string {
	if int(id) < len(msgNames) {
		return msgNames[id]
	}

	return fmt.Sprintf("unknown(%d)", uint8(id))
}

const BlockSize = 16 * 1024
const maxMessageLen = 1 << 20

type Message struct {
	ID      MessageID
	Payload []byte
}

func (m *Message) String() string {
	if m == nil {
		return "keep-alive"
	}
	return fmt.Sprintf("%s [%d bytes]", m.ID, len(m.Payload))
}

func (m *Message) Serialize() []byte {
	if m == nil {
		return make([]byte, 4)
	}
	buf := make([]byte, 5+len(m.Payload))
	binary.BigEndian.PutUint32(buf[0:4], uint32(1+len(m.Payload)))
	buf[4] = byte(m.ID)
	copy(buf[5:], m.Payload)
	return buf
}

func Read(r io.Reader) (*Message, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	if n == 0 {
		return nil, nil
	}
	if n > maxMessageLen {
		return nil, fmt.Errorf("peer: message too long (%d bytes)", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return &Message{ID: MessageID(buf[0]), Payload: buf[1:]}, nil
}

func NewRequest(index, begin, length uint32) *Message {
	return &Message{ID: MsgRequest, Payload: be32(index, begin, length)}
}

func NewCancel(index, begin, length uint32) *Message {
	return &Message{ID: MsgCancel, Payload: be32(index, begin, length)}
}

func NewHave(index uint32) *Message {
	return &Message{ID: MsgHave, Payload: be32(index)}
}

func NewPiece(index, begin uint32, block []byte) *Message {
	return &Message{ID: MsgPiece, Payload: append(be32(index, begin), block...)}
}

func be32(vals ...uint32) []byte {
	b := make([]byte, 4*len(vals))
	for i, v := range vals {
		binary.BigEndian.PutUint32(b[4*i:], v)
	}
	return b
}

func ParseHave(m *Message) (uint32, error) {
	if m.ID != MsgHave || len(m.Payload) != 4 {
		return 0, fmt.Errorf("peer: bad have message: %v", m)
	}
	return binary.BigEndian.Uint32(m.Payload), nil
}

func ParseRequest(m *Message) (index, begin, length uint32, err error) {
	if (m.ID != MsgRequest && m.ID != MsgCancel) || len(m.Payload) != 12 {
		return 0, 0, 0, fmt.Errorf("peer: bad %v message", m)
	}
	p := m.Payload
	return binary.BigEndian.Uint32(p[0:4]), binary.BigEndian.Uint32(p[4:8]),
		binary.BigEndian.Uint32(p[8:12]), nil
}

func ParsePiece(m *Message) (index, begin uint32, block []byte, err error) {
	if m.ID != MsgPiece || len(m.Payload) < 8 {
		return 0, 0, nil, fmt.Errorf("peer: bad piece message: %v", m)
	}
	p := m.Payload
	return binary.BigEndian.Uint32(p[0:4]), binary.BigEndian.Uint32(p[4:8]), p[8:], nil
}

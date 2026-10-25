package peer

import (
	"errors"
	"fmt"
	"io"
)

const protocol = "BitTorrent protocol"

type Handshake struct {
	InfoHash [20]byte
	PeerID   [20]byte
}

func (h Handshake) Serialize() []byte {
	buf := make([]byte, 49+len(protocol))
	buf[0] = byte(len(protocol))
	cur := 1
	cur += copy(buf[cur:], protocol)
	cur += 8
	cur += copy(buf[cur:], h.InfoHash[:])
	copy(buf[cur:], h.PeerID[:])
	return buf
}

func ReadHandshake(r io.Reader) (Handshake, error) {
	var lenBuf [1]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return Handshake{}, err
	}
	pstrlen := int(lenBuf[0])
	if pstrlen == 0 {
		return Handshake{}, errors.New("peer: pstrlen cannot be 0")
	}
	buf := make([]byte, pstrlen+48)
	if _, err := io.ReadFull(r, buf); err != nil {
		return Handshake{}, err
	}
	if string(buf[:pstrlen]) != protocol {
		return Handshake{}, fmt.Errorf("peer: unknown protocol %q", buf[:pstrlen])
	}
	var h Handshake
	copy(h.InfoHash[:], buf[pstrlen+8:pstrlen+28])
	copy(h.PeerID[:], buf[pstrlen+28:])
	return h, nil
}

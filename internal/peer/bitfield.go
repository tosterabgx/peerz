package peer

import (
	"fmt"
	"math/bits"
)

type Bitfield []byte

func NewBitfield(numPieces int) Bitfield {
	return make(Bitfield, (numPieces+7)/8)
}

func (b Bitfield) Has(i int) bool {
	if i < 0 || i/8 >= len(b) {
		return false
	}
	return b[i/8]>>(7-i%8)&1 != 0
}

func (b Bitfield) Set(i int) {
	if i >= 0 && i/8 < len(b) {
		b[i/8] |= 1 << (7 - i%8)
	}
}

func (b Bitfield) Count() int {
	n := 0
	for _, x := range b {
		n += bits.OnesCount8(x)
	}
	return n
}

func ParseBitfield(payload []byte, numPieces int) (Bitfield, error) {
	want := (numPieces + 7) / 8
	if len(payload) != want {
		return nil, fmt.Errorf("peer: bitfield is %d bytes, want %d", len(payload), want)
	}
	if spare := want*8 - numPieces; spare > 0 {
		if payload[want-1]&(1<<spare-1) != 0 {
			return nil, fmt.Errorf("peer: bitfield has spare bits set")
		}
	}
	return Bitfield(append([]byte(nil), payload...)), nil
}

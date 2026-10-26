package piece

import (
	"math/rand/v2"
	"net/netip"

	"github.com/tosterabgx/peerz/internal/peer"
)

const BlockSize = peer.BlockSize

type Block struct {
	Index, Begin, Length uint32
}

type pieceState uint8

const (
	stateMissing pieceState = iota
	stateActive
	stateWriting
	stateDone
)

type blockState uint8

const (
	blockMissing blockState = iota
	blockRequested
	blockReceived
)

type active struct {
	buf      []byte
	blocks   []blockState
	received int
	from     map[netip.AddrPort]struct{}
}

type Picker struct {
	pieceLen, total int64
	maxActive       int

	state   []pieceState
	avail   []int
	active  map[int]*active
	contrib map[int]map[netip.AddrPort]struct{}
	writing int

	have      peer.Bitfield
	done      int
	doneBytes int64
}

func New(numPieces int, pieceLen, total int64, maxActive int) *Picker {
	return &Picker{
		pieceLen:  pieceLen,
		total:     total,
		maxActive: maxActive,
		state:     make([]pieceState, numPieces),
		avail:     make([]int, numPieces),
		active:    make(map[int]*active),
		contrib:   make(map[int]map[netip.AddrPort]struct{}),
		have:      peer.NewBitfield(numPieces),
	}
}

func (p *Picker) pieceSize(i int) int64 { return min(p.pieceLen, p.total-int64(i)*p.pieceLen) }

func (p *Picker) numBlocks(i int) int { return int((p.pieceSize(i) + BlockSize - 1) / BlockSize) }

func (p *Picker) block(i, b int) Block {
	begin := int64(b) * BlockSize
	return Block{Index: uint32(i), Begin: uint32(begin), Length: uint32(min(BlockSize, p.pieceSize(i)-begin))}
}

func (p *Picker) AddPeer(bf peer.Bitfield) {
	for i := range p.avail {
		if bf.Has(i) {
			p.avail[i]++
		}
	}
}

func (p *Picker) RemovePeer(bf peer.Bitfield) {
	for i := range p.avail {
		if bf.Has(i) {
			p.avail[i]--
		}
	}
}

func (p *Picker) PeerHas(i int) {
	if i >= 0 && i < len(p.avail) {
		p.avail[i]++
	}
}

func (p *Picker) Needs(i int) bool {
	return i >= 0 && i < len(p.state) && p.state[i] != stateDone
}

func (p *Picker) Interesting(bf peer.Bitfield) bool {
	for i := range p.state {
		if p.state[i] != stateDone && bf.Has(i) {
			return true
		}
	}
	return false
}

func (p *Picker) Have() peer.Bitfield { return p.have }
func (p *Picker) Done() bool          { return p.done == len(p.state) }
func (p *Picker) DonePieces() int     { return p.done }
func (p *Picker) DoneBytes() int64    { return p.doneBytes }

func (p *Picker) Next(bf peer.Bitfield, n int) []Block {
	var out []Block

	for i, a := range p.active {
		if len(out) >= n {
			return out
		}
		if bf.Has(i) {
			out = p.take(i, a, out, n)
		}
	}

	for len(out) < n && len(p.active)+p.writing < p.maxActive {
		i := p.rarest(bf)
		if i < 0 {
			break
		}
		a := &active{
			buf:    make([]byte, p.pieceSize(i)),
			blocks: make([]blockState, p.numBlocks(i)),
			from:   make(map[netip.AddrPort]struct{}),
		}
		p.active[i] = a
		p.state[i] = stateActive
		out = p.take(i, a, out, n)
	}
	return out
}

func (p *Picker) take(i int, a *active, out []Block, n int) []Block {
	for b, s := range a.blocks {
		if len(out) >= n {
			break
		}
		if s == blockMissing {
			a.blocks[b] = blockRequested
			out = append(out, p.block(i, b))
		}
	}
	return out
}

func (p *Picker) rarest(bf peer.Bitfield) int {
	best, bestAvail := -1, 0
	n := len(p.state)
	start := rand.IntN(n)
	for k := range n {
		i := (start + k) % n
		if p.state[i] != stateMissing || !bf.Has(i) {
			continue
		}
		if best == -1 || p.avail[i] < bestAvail {
			best, bestAvail = i, p.avail[i]
		}
	}
	return best
}

func (p *Picker) Received(from netip.AddrPort, index, begin uint32, data []byte) (piece []byte, complete bool) {
	i := int(index)
	a, ok := p.active[i]
	if !ok || begin%BlockSize != 0 {
		return nil, false
	}
	b := int(begin / BlockSize)
	if b >= len(a.blocks) || a.blocks[b] == blockReceived || p.block(i, b).Length != uint32(len(data)) {
		return nil, false
	}
	copy(a.buf[begin:], data)
	a.blocks[b] = blockReceived
	a.received++
	a.from[from] = struct{}{}
	if a.received < len(a.blocks) {
		return nil, false
	}
	delete(p.active, i)
	p.contrib[i] = a.from
	p.state[i] = stateWriting
	p.writing++
	return a.buf, true
}

func (p *Picker) Release(blocks []Block) {
	for _, bl := range blocks {
		a, ok := p.active[int(bl.Index)]
		if !ok {
			continue
		}
		if b := int(bl.Begin / BlockSize); b < len(a.blocks) && a.blocks[b] == blockRequested {
			a.blocks[b] = blockMissing
		}
	}
}

func (p *Picker) WriteDone(i int, ok bool) (suppliers []netip.AddrPort) {
	if i < 0 || i >= len(p.state) || p.state[i] != stateWriting {
		return nil
	}
	p.writing--
	from := p.contrib[i]
	delete(p.contrib, i)
	if ok {
		p.state[i] = stateDone
		p.have.Set(i)
		p.done++
		p.doneBytes += p.pieceSize(i)
		return nil
	}
	p.state[i] = stateMissing
	for addr := range from {
		suppliers = append(suppliers, addr)
	}
	return suppliers
}

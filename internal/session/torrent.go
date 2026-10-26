package session

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/tosterabgx/peerz/internal/metainfo"
	"github.com/tosterabgx/peerz/internal/peer"
	"github.com/tosterabgx/peerz/internal/piece"
	"github.com/tosterabgx/peerz/internal/storage"
	"github.com/tosterabgx/peerz/internal/tracker"
)

type Config struct {
	DownloadDir string
	PeerID      [20]byte
	Port        uint16
	MaxPeers    int
	Pipeline    int
	MaxActive   int
	Logf        func(format string, args ...any)
}

type Stats struct {
	Name      string
	InfoHash  [20]byte
	Size      int64
	Pieces    int
	Done      int
	DoneBytes int64
	DownRate  int64 // bytes/sec
	Peers     int
	State     string // idle, downloading, paused, seeding, stopped, error
	Tracker   string // last tracker error, if any
}

type Torrent struct {
	meta  *metainfo.Torrent
	cfg   Config
	stats atomic.Pointer[Stats]
	pause chan struct{}
	add   chan []netip.AddrPort

	picker     *piece.Picker
	peers      map[netip.AddrPort]*peerConn
	known      map[netip.AddrPort]bool
	banned     map[netip.AddrPort]bool
	candidates []netip.AddrPort
	dialing    int
	paused     bool
	state      string
	trackerErr string
	fatal      error
	tickBytes  int64
	rate       int64
	left       atomic.Int64
	downloaded atomic.Int64

	ctx     context.Context
	events  chan peer.Event
	dialed  chan dialResult
	jobs    chan storage.WriteJob
	results chan storage.WriteResult
}

type peerConn struct {
	addr       netip.AddrPort
	cmds       chan *peer.Message
	cancel     context.CancelFunc
	bf         peer.Bitfield
	choked     bool
	interested bool
	inflight   map[piece.Block]struct{}
	strikes    int
	gone       bool
}

type dialResult struct {
	addr netip.AddrPort
	conn *peer.Conn
	err  error
}

type trackerResult struct {
	peers []netip.AddrPort
	err   error
}

func New(meta *metainfo.Torrent, cfg Config) *Torrent {
	if cfg.MaxPeers <= 0 {
		cfg.MaxPeers = 30
	}
	if cfg.Pipeline <= 0 {
		cfg.Pipeline = 16
	}
	if cfg.MaxActive <= 0 {
		cfg.MaxActive = 32
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	t := &Torrent{
		meta:  meta,
		cfg:   cfg,
		pause: make(chan struct{}, 1),
		add:   make(chan []netip.AddrPort, 16),
		state: "idle",
	}
	t.publish()
	return t
}

func (t *Torrent) Stats() Stats { return *t.stats.Load() }

func (t *Torrent) TogglePause() {
	select {
	case t.pause <- struct{}{}:
	default:
	}
}

func (t *Torrent) AddPeers(addrs []netip.AddrPort) { t.add <- addrs }

func (t *Torrent) logf(format string, args ...any) {
	t.cfg.Logf("[%s] "+format, append([]any{t.meta.Name}, args...)...)
}

func (t *Torrent) Run(ctx context.Context) error {
	store, err := storage.Open(t.cfg.DownloadDir, t.meta)
	if err != nil {
		t.state = "error"
		t.publish()
		return err
	}
	defer store.Close()

	n := len(t.meta.Pieces)
	t.picker = piece.New(n, t.meta.PieceLength, t.meta.Length, t.cfg.MaxActive)
	t.peers = make(map[netip.AddrPort]*peerConn)
	t.known = make(map[netip.AddrPort]bool)
	t.banned = make(map[netip.AddrPort]bool)
	t.left.Store(t.meta.Length)

	t.ctx = ctx
	t.events = make(chan peer.Event, 256)
	t.dialed = make(chan dialResult, t.cfg.MaxPeers)
	t.jobs = make(chan storage.WriteJob, t.cfg.MaxActive)
	t.results = make(chan storage.WriteResult, t.cfg.MaxActive)

	writerDone := make(chan struct{})
	go func() {
		store.RunWriter(t.jobs, t.results)
		close(writerDone)
	}()
	defer func() {
		close(t.jobs)
		<-writerDone
	}()

	trackerResults := make(chan trackerResult, 1)
	go t.trackerLoop(ctx, trackerResults)

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	t.state = "downloading"
	t.publish()

	for t.fatal == nil {
		select {
		case <-ctx.Done():
			t.state = "stopped"
			t.publish()
			t.announceStopped()
			return nil
		case addrs := <-t.add:
			t.addCandidates(addrs)
		case r := <-trackerResults:
			t.trackerErr = ""
			if r.err != nil {
				t.trackerErr = r.err.Error()
				t.logf("tracker: %v", r.err)
			}
			t.addCandidates(r.peers)
		case d := <-t.dialed:
			t.dialing--
			if d.err != nil {
				t.logf("dial %v: %v", d.addr, d.err)
				delete(t.known, d.addr)
			} else {
				t.startPeer(d.conn)
			}
			t.connectMore()
		case ev := <-t.events:
			t.handleEvent(ev)
		case r := <-t.results:
			t.handleWrite(r)
		case <-t.pause:
			t.paused = !t.paused
			t.refreshState()
			t.fillAll()
		case <-tick.C:
			t.rate = (t.rate + t.tickBytes) / 2
			t.tickBytes = 0
			t.connectMore()
			t.publish()
		}
	}
	t.state = "error"
	t.publish()
	return t.fatal
}

func (t *Torrent) refreshState() {
	switch {
	case t.picker.Done():
		t.state = "seeding"
	case t.paused:
		t.state = "paused"
	default:
		t.state = "downloading"
	}
	t.publish()
}

func (t *Torrent) publish() {
	s := &Stats{
		Name:     t.meta.Name,
		InfoHash: t.meta.InfoHash,
		Size:     t.meta.Length,
		Pieces:   len(t.meta.Pieces),
		DownRate: t.rate,
		Peers:    len(t.peers),
		State:    t.state,
		Tracker:  t.trackerErr,
	}
	if t.picker != nil {
		s.Done = t.picker.DonePieces()
		s.DoneBytes = t.picker.DoneBytes()
	}
	t.stats.Store(s)
}

func (t *Torrent) addCandidates(addrs []netip.AddrPort) {
	for _, a := range addrs {
		if !t.known[a] && !t.banned[a] && a.IsValid() && a.Port() != 0 {
			t.known[a] = true
			t.candidates = append(t.candidates, a)
		}
	}
	t.connectMore()
}

func (t *Torrent) connectMore() {
	if t.picker.Done() {
		return // TODO: add seeding
	}
	for len(t.candidates) > 0 && len(t.peers)+t.dialing < t.cfg.MaxPeers {
		addr := t.candidates[0]
		t.candidates = t.candidates[1:]
		t.dialing++
		go func() {
			c, err := peer.Dial(t.ctx, addr, t.meta.InfoHash, t.cfg.PeerID)
			select {
			case t.dialed <- dialResult{addr, c, err}:
			case <-t.ctx.Done():
				if c != nil {
					c.Close()
				}
			}
		}()
	}
}

func (t *Torrent) startPeer(c *peer.Conn) {
	pctx, cancel := context.WithCancel(t.ctx)
	pc := &peerConn{
		addr:     c.Addr,
		cmds:     make(chan *peer.Message, t.cfg.Pipeline+64),
		cancel:   cancel,
		bf:       peer.NewBitfield(len(t.meta.Pieces)),
		choked:   true,
		inflight: make(map[piece.Block]struct{}),
	}
	t.peers[pc.addr] = pc
	go c.Run(pctx, pc.cmds, t.events)
	t.logf("connected to %v", pc.addr)
	if t.picker.DonePieces() > 0 {
		bf := t.picker.Have()
		t.send(pc, &peer.Message{ID: peer.MsgBitfield, Payload: append([]byte(nil), bf...)})
	}
}

func (t *Torrent) send(pc *peerConn, m *peer.Message) bool {
	if pc.gone {
		return false
	}
	select {
	case pc.cmds <- m:
		return true
	default:
		t.dropPeer(pc, errors.New("send queue full"))
		return false
	}
}

func (t *Torrent) dropPeer(pc *peerConn, err error) {
	if pc.gone {
		return
	}
	pc.gone = true
	pc.cancel()
	t.releaseInflight(pc)
	t.picker.RemovePeer(pc.bf)
	delete(t.peers, pc.addr)
	delete(t.known, pc.addr)
	t.logf("dropped %v: %v", pc.addr, err)
	t.connectMore()
}

func (t *Torrent) releaseInflight(pc *peerConn) {
	blocks := make([]piece.Block, 0, len(pc.inflight))
	for b := range pc.inflight {
		blocks = append(blocks, b)
	}
	t.picker.Release(blocks)
	clear(pc.inflight)
}

func (t *Torrent) handleEvent(ev peer.Event) {
	pc, ok := t.peers[ev.Peer]
	if !ok {
		return
	}
	if ev.Err != nil {
		t.dropPeer(pc, ev.Err)
		return
	}
	m := ev.Msg
	switch m.ID {
	case peer.MsgBitfield:
		bf, err := peer.ParseBitfield(m.Payload, len(t.meta.Pieces))
		if err != nil {
			t.dropPeer(pc, err)
			return
		}
		t.picker.RemovePeer(pc.bf)
		pc.bf = bf
		t.picker.AddPeer(bf)
		if !pc.interested && t.picker.Interesting(bf) {
			t.setInterested(pc)
		}
	case peer.MsgHave:
		i, err := peer.ParseHave(m)
		if err != nil || int(i) >= len(t.meta.Pieces) {
			t.dropPeer(pc, fmt.Errorf("bad have: %v", err))
			return
		}
		if !pc.bf.Has(int(i)) {
			pc.bf.Set(int(i))
			t.picker.PeerHas(int(i))
		}
		if !pc.interested && t.picker.Needs(int(i)) {
			t.setInterested(pc)
		}
	case peer.MsgChoke:
		pc.choked = true
		t.releaseInflight(pc)
	case peer.MsgUnchoke:
		pc.choked = false
	case peer.MsgPiece:
		index, begin, block, err := peer.ParsePiece(m)
		if err != nil {
			t.dropPeer(pc, err)
			return
		}
		delete(pc.inflight, piece.Block{Index: index, Begin: begin, Length: uint32(len(block))})
		t.tickBytes += int64(len(block))
		if data, complete := t.picker.Received(pc.addr, index, begin, block); complete {
			t.jobs <- storage.WriteJob{Index: int(index), Data: data}
		}
	}
	t.fill(pc)
}

func (t *Torrent) setInterested(pc *peerConn) {
	if t.send(pc, &peer.Message{ID: peer.MsgInterested}) {
		pc.interested = true
	}
}

func (t *Torrent) fill(pc *peerConn) {
	if pc.gone || pc.choked || !pc.interested || t.paused {
		return
	}
	want := t.cfg.Pipeline - len(pc.inflight)
	if want <= 0 {
		return
	}
	blocks := t.picker.Next(pc.bf, want)
	for k, b := range blocks {
		pc.inflight[b] = struct{}{}
		if !t.send(pc, peer.NewRequest(b.Index, b.Begin, b.Length)) {
			t.picker.Release(blocks[k+1:])
			return
		}
	}
}

func (t *Torrent) fillAll() {
	for _, pc := range t.peers {
		t.fill(pc)
	}
}

func (t *Torrent) handleWrite(r storage.WriteResult) {
	if r.Err != nil {
		t.picker.WriteDone(r.Index, false)
		t.fatal = fmt.Errorf("writing piece %d: %w", r.Index, r.Err)
		return
	}
	suppliers := t.picker.WriteDone(r.Index, r.OK)
	if r.OK {
		t.left.Store(t.meta.Length - t.picker.DoneBytes())
		t.downloaded.Store(t.picker.DoneBytes())
		for _, pc := range t.peers {
			select {
			case pc.cmds <- peer.NewHave(uint32(r.Index)):
			default:
			}
		}
		if t.picker.Done() {
			t.logf("download complete")
			for _, pc := range t.peers {
				t.send(pc, &peer.Message{ID: peer.MsgNotInterested})
			}
			t.refreshState()
		}
	} else {
		t.logf("piece %d failed hash check (suppliers %v)", r.Index, suppliers)
		if len(suppliers) == 1 {
			if pc := t.peers[suppliers[0]]; pc != nil {
				if pc.strikes++; pc.strikes >= 2 {
					t.banned[pc.addr] = true
					t.dropPeer(pc, errors.New("banned: sent corrupt data"))
				}
			}
		}
	}
	t.fillAll()
}

func (t *Torrent) trackerURLs() []string {
	seen := map[string]bool{}
	var urls []string
	for _, u := range append([]string{t.meta.Announce}, flatten(t.meta.AnnounceList)...) {
		if u != "" && !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}
	return urls
}

func flatten(tiers [][]string) []string {
	var out []string
	for _, tier := range tiers {
		out = append(out, tier...)
	}
	return out
}

func (t *Torrent) trackerLoop(ctx context.Context, out chan<- trackerResult) {
	urls := t.trackerURLs()
	if len(urls) == 0 {
		return
	}
	event := tracker.EventStarted
	for {
		resp, err := tracker.AnnounceAny(ctx, urls, tracker.Request{
			InfoHash:   t.meta.InfoHash,
			PeerID:     t.cfg.PeerID,
			Port:       t.cfg.Port,
			Downloaded: t.downloaded.Load(),
			Left:       t.left.Load(),
			Event:      event,
			NumWant:    200,
		})
		r, wait := trackerResult{err: err}, time.Minute
		if err == nil {
			event = tracker.EventNone
			r.peers = resp.Peers
			wait = max(resp.Interval, time.Minute)
		}
		select {
		case out <- r:
		case <-ctx.Done():
			return
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return
		}
	}
}

func (t *Torrent) announceStopped() {
	urls := t.trackerURLs()
	if len(urls) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tracker.AnnounceAny(ctx, urls, tracker.Request{
		InfoHash:   t.meta.InfoHash,
		PeerID:     t.cfg.PeerID,
		Port:       t.cfg.Port,
		Downloaded: t.downloaded.Load(),
		Left:       t.left.Load(),
		Event:      tracker.EventStopped,
	})
}

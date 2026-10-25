package tracker

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"time"
)

type Event uint32

const (
	EventNone Event = iota
	EventCompleted
	EventStarted
	EventStopped
)

func (e Event) String() string {
	switch e {
	case EventCompleted:
		return "completed"
	case EventStarted:
		return "started"
	case EventStopped:
		return "stopped"
	}
	return ""
}

type Request struct {
	InfoHash   [20]byte
	PeerID     [20]byte
	Port       uint16
	Uploaded   int64
	Downloaded int64
	Left       int64
	Event      Event
	NumWant    int // 0 means let the tracker decide
}

type Response struct {
	Interval time.Duration
	Seeders  int
	Leechers int
	Peers    []netip.AddrPort
}

func Announce(ctx context.Context, rawURL string, req Request) (*Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http", "https":
		return announceHTTP(ctx, u, req)
	case "udp":
		return announceUDP(ctx, u, req)
	default:
		return nil, fmt.Errorf("tracker: unsupported scheme %q", u.Scheme)
	}
}

func AnnounceAny(ctx context.Context, urls []string, req Request) (*Response, error) {
	var errs []error
	for _, u := range urls {
		resp, err := Announce(ctx, u, req)
		if err == nil {
			return resp, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", u, err))
		if ctx.Err() != nil {
			break
		}
	}
	if len(errs) == 0 {
		return nil, errors.New("tracker: no trackers to announce to")
	}
	return nil, errors.Join(errs...)
}

func NewPeerID() [20]byte {
	var id [20]byte
	copy(id[:], "-PZ0001-")
	rand.Read(id[8:])
	return id
}

const (
	peerLen4 = 6
	peerLen6 = 18
)

func parseCompactPeers(b []byte, size int) ([]netip.AddrPort, error) {
	if len(b)%size != 0 {
		return nil, fmt.Errorf("tracker: compact peers length %d not a multiple of %d", len(b), size)
	}
	peers := make([]netip.AddrPort, 0, len(b)/size)
	for i := 0; i < len(b); i += size {
		var addr netip.Addr
		if size == peerLen4 {
			addr = netip.AddrFrom4([4]byte(b[i : i+4]))
		} else {
			addr = netip.AddrFrom16([16]byte(b[i : i+16]))
		}
		port := binary.BigEndian.Uint16(b[i+size-2 : i+size])
		peers = append(peers, netip.AddrPortFrom(addr, port))
	}
	return peers, nil
}

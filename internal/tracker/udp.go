package tracker

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"time"
)

const (
	udpProtocolID = 0x41727101980

	actionConnect  = 0
	actionAnnounce = 1
	actionError    = 3

	udpRetries     = 3
	udpBaseTimeout = 15 * time.Second
)

func announceUDP(ctx context.Context, u *url.URL, req Request) (*Response, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", u.Host)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	connID, err := udpConnect(ctx, conn)
	if err != nil {
		return nil, err
	}

	peerLen := peerLen4
	if ra, ok := conn.RemoteAddr().(*net.UDPAddr); ok && ra.IP.To4() == nil {
		peerLen = peerLen6
	}
	return udpAnnounce(ctx, conn, connID, req, peerLen)
}

func udpConnect(ctx context.Context, conn net.Conn) (uint64, error) {
	tid := rand.Uint32()
	pkt := make([]byte, 16)
	binary.BigEndian.PutUint64(pkt[0:8], udpProtocolID)
	binary.BigEndian.PutUint32(pkt[8:12], actionConnect)
	binary.BigEndian.PutUint32(pkt[12:16], tid)

	resp, err := udpRoundTrip(ctx, conn, pkt, actionConnect, tid, 16)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(resp[8:16]), nil
}

func udpAnnounce(ctx context.Context, conn net.Conn, connID uint64, req Request, peerLen int) (*Response, error) {
	tid := rand.Uint32()
	numWant := int32(-1)
	if req.NumWant > 0 {
		numWant = int32(req.NumWant)
	}

	pkt := make([]byte, 98)
	binary.BigEndian.PutUint64(pkt[0:8], connID)
	binary.BigEndian.PutUint32(pkt[8:12], actionAnnounce)
	binary.BigEndian.PutUint32(pkt[12:16], tid)
	copy(pkt[16:36], req.InfoHash[:])
	copy(pkt[36:56], req.PeerID[:])
	binary.BigEndian.PutUint64(pkt[56:64], uint64(req.Downloaded))
	binary.BigEndian.PutUint64(pkt[64:72], uint64(req.Left))
	binary.BigEndian.PutUint64(pkt[72:80], uint64(req.Uploaded))
	binary.BigEndian.PutUint32(pkt[80:84], uint32(req.Event))
	binary.BigEndian.PutUint32(pkt[88:92], rand.Uint32())
	binary.BigEndian.PutUint32(pkt[92:96], uint32(numWant))
	binary.BigEndian.PutUint16(pkt[96:98], req.Port)

	resp, err := udpRoundTrip(ctx, conn, pkt, actionAnnounce, tid, 20)
	if err != nil {
		return nil, err
	}
	peers, err := parseCompactPeers(resp[20:], peerLen)
	if err != nil {
		return nil, err
	}
	return &Response{
		Interval: time.Duration(binary.BigEndian.Uint32(resp[8:12])) * time.Second,
		Leechers: int(binary.BigEndian.Uint32(resp[12:16])),
		Seeders:  int(binary.BigEndian.Uint32(resp[16:20])),
		Peers:    peers,
	}, nil
}

func udpRoundTrip(ctx context.Context, conn net.Conn, pkt []byte, action, tid uint32, minLen int) ([]byte, error) {
	buf := make([]byte, 64*1024)
	for attempt := 0; attempt < udpRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := conn.Write(pkt); err != nil {
			return nil, err
		}

		deadline := time.Now().Add(udpBaseTimeout << attempt)
		if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
			deadline = dl
		}
		conn.SetReadDeadline(deadline)

		for {
			n, err := conn.Read(buf)
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				break
			}
			if err != nil {
				return nil, err
			}
			if n < 8 || binary.BigEndian.Uint32(buf[4:8]) != tid {
				continue
			}
			gotAction := binary.BigEndian.Uint32(buf[0:4])
			if gotAction == actionError {
				return nil, fmt.Errorf("tracker: %s", buf[8:n])
			}
			if gotAction != action || n < minLen {
				return nil, fmt.Errorf("tracker: unexpected reply (action %d, %d bytes)", gotAction, n)
			}
			return append([]byte(nil), buf[:n]...), nil
		}
	}
	return nil, errors.New("tracker: no response from UDP tracker")
}

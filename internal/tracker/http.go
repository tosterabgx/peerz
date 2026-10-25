package tracker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"time"

	"github.com/tosterabgx/peerz/internal/bencode"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

func announceHTTP(ctx context.Context, u *url.URL, req Request) (*Response, error) {
	q := u.Query()
	q.Set("info_hash", string(req.InfoHash[:]))
	q.Set("peer_id", string(req.PeerID[:]))
	q.Set("port", strconv.Itoa(int(req.Port)))
	q.Set("uploaded", strconv.FormatInt(req.Uploaded, 10))
	q.Set("downloaded", strconv.FormatInt(req.Downloaded, 10))
	q.Set("left", strconv.FormatInt(req.Left, 10))
	q.Set("compact", "1")
	if ev := req.Event.String(); ev != "" {
		q.Set("event", ev)
	}
	if req.NumWant > 0 {
		q.Set("numwant", strconv.Itoa(req.NumWant))
	}

	full := *u
	full.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, full.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracker: HTTP %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return parseHTTPResponse(body)
}

func parseHTTPResponse(body []byte) (*Response, error) {
	v, _, err := bencode.Decode(body)
	if err != nil {
		return nil, fmt.Errorf("tracker: bad response: %w", err)
	}
	d, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("tracker: response is not a dictionary")
	}
	if reason, ok := d["failure reason"].(string); ok {
		return nil, fmt.Errorf("tracker: %s", reason)
	}

	r := &Response{}
	if iv, ok := d["interval"].(int64); ok {
		r.Interval = time.Duration(iv) * time.Second
	}
	if n, ok := d["complete"].(int64); ok {
		r.Leechers = int(n)
	}

	switch p := d["peers"].(type) {
	case string:
		peers, err := parseCompactPeers([]byte(p), peerLen4)
		if err != nil {
			return nil, err
		}
		r.Peers = append(r.Peers, peers...)
	case []any:
		for _, e := range p {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			ip, _ := m["ip"].(string)
			port, _ := m["port"].(int64)
			addr, err := netip.ParseAddr(ip)
			if err != nil || port <= 0 || port > 65535 {
				continue
			}
			r.Peers = append(r.Peers, netip.AddrPortFrom(addr, uint16(port)))
		}
	}

	if p6, ok := d["peers6"].(string); ok {
		peers, err := parseCompactPeers([]byte(p6), peerLen6)
		if err != nil {
			return nil, err
		}
		r.Peers = append(r.Peers, peers...)
	}
	return r, nil
}

package peer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"
)

const (
	handshakeTimeout  = 10 * time.Second
	readTimeout       = 3 * time.Minute
	writeTimeout      = 30 * time.Second
	keepAliveInterval = 90 * time.Second
)

var ErrInfoHashMismatch = errors.New("peer: info hash mismatch")

type Conn struct {
	Addr   netip.AddrPort
	PeerID [20]byte
	conn   net.Conn
}

type Event struct {
	Peer netip.AddrPort
	Msg  *Message
	Err  error
}

func Dial(ctx context.Context, addr netip.AddrPort, infoHash, peerID [20]byte) (*Conn, error) {
	var d net.Dialer
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	nc, err := d.DialContext(ctx, "tcp", addr.String())
	if err != nil {
		return nil, err
	}
	c, err := handshake(nc, addr, infoHash, peerID)
	if err != nil {
		nc.Close()
		return nil, err
	}
	return c, nil
}

func handshake(nc net.Conn, addr netip.AddrPort, infoHash, peerID [20]byte) (*Conn, error) {
	nc.SetDeadline(time.Now().Add(handshakeTimeout))
	defer nc.SetDeadline(time.Time{})

	if _, err := nc.Write(Handshake{InfoHash: infoHash, PeerID: peerID}.Serialize()); err != nil {
		return nil, err
	}
	h, err := ReadHandshake(nc)
	if err != nil {
		return nil, fmt.Errorf("peer: handshake: %w", err)
	}
	if h.InfoHash != infoHash {
		return nil, ErrInfoHashMismatch
	}
	return &Conn{Addr: addr, PeerID: h.PeerID, conn: nc}, nil
}

func (c *Conn) Send(m *Message) error {
	c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	_, err := c.conn.Write(m.Serialize())
	return err
}

func (c *Conn) Receive() (*Message, error) {
	c.conn.SetReadDeadline(time.Now().Add(readTimeout))
	return Read(c.conn)
}

func (c *Conn) Close() error { return c.conn.Close() }

func (c *Conn) Run(ctx context.Context, cmds <-chan *Message, events chan<- Event) {
	inner, cancel := context.WithCancel(ctx)
	defer cancel()
	defer c.Close()

	go func() {
		<-inner.Done()
		c.conn.SetDeadline(time.Now())
	}()

	readErr := make(chan error, 1)
	go func() {
		for {
			msg, err := c.Receive()
			if err != nil {
				readErr <- err
				return
			}
			if msg == nil {
				continue
			}
			select {
			case events <- Event{Peer: c.Addr, Msg: msg}:
			case <-inner.Done():
				return
			}
		}
	}()

	keepAlive := time.NewTicker(keepAliveInterval)
	defer keepAlive.Stop()

	err := func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case err := <-readErr:
				return err
			case m, ok := <-cmds:
				if !ok {
					return errors.New("peer: closed by us")
				}
				if err := c.Send(m); err != nil {
					return err
				}
			case <-keepAlive.C:
				if err := c.Send(nil); err != nil {
					return err
				}
			}
		}
	}()

	select {
	case events <- Event{Peer: c.Addr, Err: err}:
	case <-ctx.Done():
	}
}

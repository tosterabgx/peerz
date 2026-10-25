package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/tosterabgx/peerz/internal/metainfo"
	"github.com/tosterabgx/peerz/internal/tracker"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: trackercheck <file.torrent>")
		os.Exit(1)
	}
	t, err := metainfo.Load(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	urls := []string{t.Announce}
	for _, tier := range t.AnnounceList {
		urls = append(urls, tier...)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := tracker.AnnounceAny(ctx, urls, tracker.Request{
		InfoHash: t.InfoHash,
		PeerID:   tracker.NewPeerID(),
		Port:     6881,
		Left:     t.Length,
		Event:    tracker.EventStarted,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("%d seeders, %d leechers, re-announce every %v\n", resp.Seeders, resp.Leechers, resp.Interval)
	for _, p := range resp.Peers {
		fmt.Println(p)
	}
}

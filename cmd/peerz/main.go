package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/tosterabgx/peerz/internal/metainfo"
	"github.com/tosterabgx/peerz/internal/session"
	"github.com/tosterabgx/peerz/internal/tracker"
	"github.com/tosterabgx/peerz/internal/ui"
)

type backend struct {
	torrents []*session.Torrent
}

func (b *backend) Snapshot() []ui.TorrentView {
	views := make([]ui.TorrentView, 0, len(b.torrents))
	for _, t := range b.torrents {
		s := t.Stats()
		var progress float64
		if s.Size > 0 {
			progress = float64(s.DoneBytes) / float64(s.Size)
		}
		views = append(views, ui.TorrentView{
			ID:       hex.EncodeToString(s.InfoHash[:]),
			Name:     s.Name,
			Size:     s.Size,
			Pieces:   s.Pieces,
			Progress: progress,
			DownRate: s.DownRate,
			Peers:    s.Peers,
			State:    s.State,
		})
	}
	return views
}

func (b *backend) TogglePause(id string) {
	for _, t := range b.torrents {
		s := t.Stats()
		if hex.EncodeToString(s.InfoHash[:]) == id {
			t.TogglePause()
		}
	}
}

func main() {
	dir := flag.String("dir", ".", "download directory")
	port := flag.Uint("port", 6881, "port to report to trackers")
	logPath := flag.String("log", "", "write a debug log to this file")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: peerz [-dir path] [-log file] <file.torrent>...")
		os.Exit(1)
	}

	logf := func(string, ...any) {}
	if *logPath != "" {
		f, err := os.Create(*logPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		defer f.Close()
		logf = log.New(f, "", log.Ltime|log.Lmicroseconds).Printf
	}

	cfg := session.Config{
		DownloadDir: *dir,
		PeerID:      tracker.NewPeerID(),
		Port:        uint16(*port),
		Logf:        logf,
	}
	b := &backend{}
	for _, path := range flag.Args() {
		meta, err := metainfo.Load(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		b.torrents = append(b.torrents, session.New(meta, cfg))
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for _, t := range b.torrents {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := t.Run(ctx); err != nil {
				logf("%s: %v", t.Stats().Name, err)
			}
		}()
	}

	_, err := tea.NewProgram(ui.New(b)).Run()
	cancel()
	wg.Wait()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

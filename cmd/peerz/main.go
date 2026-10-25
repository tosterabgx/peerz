package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/tosterabgx/peerz/internal/metainfo"
	"github.com/tosterabgx/peerz/internal/ui"
)

type staticBackend struct {
	views []ui.TorrentView
}

func (b *staticBackend) Snapshot() []ui.TorrentView {
	return slices.Clone(b.views)
}

func (b *staticBackend) TogglePause(id string) {
	for i := range b.views {
		if b.views[i].ID != id {
			continue
		}
		if b.views[i].State == "paused" {
			b.views[i].State = "idle"
		} else {
			b.views[i].State = "paused"
		}
	}
}

func main() {
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: peerz <file.torrent>")
		os.Exit(1)
	}

	backend := &staticBackend{}
	for _, path := range os.Args[1:] {
		t, err := metainfo.Load(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		backend.views = append(backend.views, ui.TorrentView{
			ID:     hex.EncodeToString(t.InfoHash[:]),
			Name:   t.Name,
			Size:   t.Length,
			Pieces: len(t.Pieces),
			State:  "idle",
		})
	}

	if _, err := tea.NewProgram(ui.New(backend)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

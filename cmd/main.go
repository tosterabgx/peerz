package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tosterabgx/peerz/internal/metainfo"
)

func main() {
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: peerz <file.torrent>")
		os.Exit(1)
	}

	_, err := metainfo.Load(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	// TODO: add ui

	// p := tea.NewProgram(ui.New)
	// if _, err := p.Run(); err != nil {
	// 	fmt.Fprintln(os.Stderr, "error:", err)
	// 	os.Exit(1)
	// }
}

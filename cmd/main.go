package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: peerz <file.torrent>")
		os.Exit(1)
	}

}

# peerz

A TUI BitTorrent client, written in Go.

![peerz downloading two torrents](demo.gif)

> peerz can download public torrents end to end, but it's young.
> It does not support DHT, PEX or magnet links for now.

## Install

Download the binary for your system from the
[Releases page](https://github.com/tosterabgx/peerz/releases).

> btw, you can test it on torrents from testdata folder!

## Features

- Handles single-file and multi-file `.torrent` files
- Announces to HTTP(S) and UDP trackers
- Downloads from up to 30 peers at once
- Rarest-first piece selection
- Bans peers that repeatedly send corrupt data
- Multiple torrents at once
- A nice UI!

## Limitations

- No resuming
- No uploading or seeding
- DHT, PEX or magnet links do not work yet
- Web seeds are not used yet


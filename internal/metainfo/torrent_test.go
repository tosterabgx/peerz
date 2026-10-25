package metainfo

import (
	"encoding/hex"
	"errors"
	"os"
	"testing"
)

const wantInfoHash = "5b1e0d988fc7a0c9e99bd852071681a59974b39f"

func TestLoadSample(t *testing.T) {
	tor, err := Load("../../testdata/sample.torrent")
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("no testdata/sample.torrent")
	}
	if err != nil {
		t.Fatal(err)
	}

	if got := hex.EncodeToString(tor.InfoHash[:]); got != wantInfoHash {
		t.Errorf("info hash = %s, want %s", got, wantInfoHash)
	}

	wantPieces := (tor.Length + tor.PieceLength - 1) / tor.PieceLength
	if int64(len(tor.Pieces)) != wantPieces {
		t.Errorf("got %d pieces, want %d", len(tor.Pieces), wantPieces)
	}

	last := len(tor.Pieces) - 1
	if size := tor.PieceSize(last); size <= 0 || size > tor.PieceLength {
		t.Errorf("last piece size %d out of range", size)
	}
}

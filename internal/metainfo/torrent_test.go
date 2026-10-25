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

func TestParseMultiFile(t *testing.T) {
	pieces := string(make([]byte, 20))
	data := []byte("d4:infod5:filesld6:lengthi3e4:pathl3:dir5:a.txteed6:lengthi5e4:pathl5:b.txteee" +
		"4:name4:root12:piece lengthi16e6:pieces20:" + pieces + "ee")

	tor, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(tor.Files) != 2 || tor.Length != 8 {
		t.Fatalf("files=%d length=%d, want 2 and 8", len(tor.Files), tor.Length)
	}
	if p := tor.Files[0].Path; len(p) != 2 || p[0] != "dir" || p[1] != "a.txt" {
		t.Errorf("first file path = %v", p)
	}
}

func TestParseRejectsMssingInfo(t *testing.T) {
	if _, err := Parse([]byte("d8:announce3:urle")); err == nil {
		t.Fatal("expected error for missing info dict")
	}
}

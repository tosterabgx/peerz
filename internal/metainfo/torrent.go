package metainfo

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"os"

	"github.com/tosterabgx/peerz/internal/bencode"
)

type File struct {
	Path   []string
	Length int64
}

type Torrent struct {
	Announce     string
	AnnounceList [][]string
	Name         string
	PieceLength  int64
	Pieces       [][20]byte
	Files        []File
	Length       int64
	InfoHash     [20]byte
}

func (t *Torrent) PieceSize(i int) int64 {
	begin := int64(i) * t.PieceLength
	end := begin + t.PieceLength
	if end > t.Length {
		end = t.Length
	}

	return end - begin
}

func Parse(data []byte) (*Torrent, error) {
	v, rawInfo, err := bencode.Decode(data)
	if err != nil {
		return nil, err
	}

	root, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("metainfo: root is not a dictionary")
	}
	info, ok := root["info"].(map[string]any)
	if !ok || rawInfo == nil {
		return nil, errors.New("metainfo: missing info dictionary")
	}

	t := &Torrent{InfoHash: sha1.Sum(rawInfo)}
	t.Announce, _ = root["announce"].(string)
	t.Name, _ = info["name"].(string)
	t.PieceLength, _ = info["piece length"].(int64)
	if t.PieceLength <= 0 {
		return nil, errors.New("metainfo: invalid piece length")
	}

	if tiers, ok := root["announce-list"].([]any); ok {
		for _, tier := range tiers {
			var urls []string
			for _, u := range tier.([]any) {
				if s, ok := u.(string); ok {
					urls = append(urls, s)
				}
			}
			t.AnnounceList = append(t.AnnounceList, urls)
		}
	}

	pieces, _ := info["pieces"].(string)
	if len(pieces)%20 != 0 {
		return nil, fmt.Errorf("metainfo: pieces length %d not a multiple of 20", len(pieces))
	}
	for i := 0; i < len(pieces); i += 20 {
		var h [20]byte
		copy(h[:], pieces[i:i+20])
		t.Pieces = append(t.Pieces, h)
	}

	if length, ok := info["length"].(int64); ok {
		t.Files = []File{{Path: []string{t.Name}, Length: length}}
	} else {
		for _, f := range info["files"].([]any) {
			fm, ok := f.(map[string]any)
			if !ok {
				return nil, errors.New("metainfo: bad file entry")
			}
			var file File
			file.Path = []string{t.Name}
			file.Length, _ = fm["length"].(int64)
			for _, p := range fm["path"].([]any) {
				if s, ok := p.(string); ok {
					file.Path = append(file.Path, s)
				}
			}
			t.Files = append(t.Files, file)
		}
	}

	for _, f := range t.Files {
		t.Length += f.Length
	}
	return t, nil
}

func Load(path string) (*Torrent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

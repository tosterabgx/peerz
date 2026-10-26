package storage

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tosterabgx/peerz/internal/metainfo"
)

type file struct {
	f      *os.File
	offset int64
	length int64
}

type Storage struct {
	files    []file
	hashes   [][20]byte
	pieceLen int64
	total    int64
}

func Open(dir string, t *metainfo.Torrent) (*Storage, error) {
	s := &Storage{hashes: t.Pieces, pieceLen: t.PieceLength, total: t.Length}
	var off int64
	for _, tf := range t.Files {
		rel := filepath.Join(tf.Path...)
		if len(tf.Path) == 0 || !filepath.IsLocal(rel) {
			s.Close()
			return nil, fmt.Errorf("storage: unsafe path %q", rel)
		}
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			s.Close()
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
		if err != nil {
			s.Close()
			return nil, err
		}
		if err := f.Truncate(tf.Length); err != nil {
			f.Close()
			s.Close()
			return nil, err
		}
		s.files = append(s.files, file{f: f, offset: off, length: tf.Length})
		off += tf.Length
	}
	return s, nil
}

func (s *Storage) Close() error {
	var errs []error
	for _, f := range s.files {
		errs = append(errs, f.f.Sync(), f.f.Close())
	}
	return errors.Join(errs...)
}

func (s *Storage) PieceSize(i int) int64 {
	return min(s.pieceLen, s.total-int64(i)*s.pieceLen)
}

func (s *Storage) span(p []byte, off int64, fn func(f *os.File, part []byte, fileOff int64) error) error {
	for _, f := range s.files {
		if len(p) == 0 {
			return nil
		}
		end := f.offset + f.length
		if off >= end {
			continue
		}
		n := min(int64(len(p)), end-off)
		if err := fn(f.f, p[:n], off-f.offset); err != nil {
			return err
		}
		p, off = p[n:], off+n
	}
	if len(p) != 0 {
		return errors.New("storage: acess past end of torrent")
	}
	return nil
}

func (s *Storage) WriteAt(p []byte, off int64) error {
	return s.span(p, off, func(f *os.File, part []byte, fo int64) error {
		_, err := f.WriteAt(part, fo)
		return err
	})
}

func (s *Storage) ReadAt(p []byte, off int64) error {
	return s.span(p, off, func(f *os.File, part []byte, fo int64) error {
		_, err := f.ReadAt(part, fo)
		return err
	})
}

func (s *Storage) Verify(i int) (bool, error) {
	buf := make([]byte, s.PieceSize(i))
	if err := s.ReadAt(buf, int64(i)*s.pieceLen); err != nil {
		return false, err
	}
	return sha1.Sum(buf) == s.hashes[i], nil
}

type WriteJob struct {
	Index int
	Data  []byte
}

type WriteResult struct {
	Index int
	OK    bool
	Err   error
}

func (s *Storage) RunWriter(jobs <-chan WriteJob, results chan<- WriteResult) {
	for job := range jobs {
		r := WriteResult{Index: job.Index}
		switch {
		case int64(len(job.Data)) != s.PieceSize(job.Index):
			r.Err = fmt.Errorf("storage: piece %d has wrong size %d", job.Index, len(job.Data))
		case sha1.Sum(job.Data) != s.hashes[job.Index]:
		default:
			r.Err = s.WriteAt(job.Data, int64(job.Index)*s.pieceLen)
			r.OK = r.Err == nil
		}
		results <- r
	}
}

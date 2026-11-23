package archive

import (
	"errors"
	"io"
	"os"
	"syscall"
)

const spoolPattern = "cockpit-export-*"

var errNotRegular = errors.New("not a regular file")

type Spool struct{ *os.File }

func OpenSpool() (*Spool, error) {
	f, err := os.CreateTemp("", spoolPattern)
	if err != nil {
		return nil, err
	}
	if err := os.Remove(f.Name()); err != nil {
		f.Close()
		return nil, err
	}
	return &Spool{File: f}, nil
}

func (s *Spool) Rewind() (int64, error) {
	size, err := s.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	_, err = s.Seek(0, io.SeekStart)
	return size, err
}

func (s *Spool) Reset() error {
	if _, err := s.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return s.Truncate(0)
}

func openRegular(root *os.Root, rel string) (*os.File, error) {
	f, err := root.OpenFile(rel, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = errNotRegular
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func readRegular(root *os.Root, rel string, limit int64) ([]byte, error) {
	f, err := openRegular(root, rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("a bundle file exceeds the size limit of the archive")
	}
	return data, nil
}

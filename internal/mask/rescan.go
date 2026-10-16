package mask

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"syscall"
)

type Report struct {
	Left    []string
	Scanned int
}

var errNotRegular = errors.New("not a regular file")

func Rescan(dir string, mask func(string) string) (Report, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Report{}, fmt.Errorf("cannot open directory to rescan: %w", err)
	}
	defer root.Close()
	var r Report
	err = fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil && path == ".":
			return err
		case err != nil:
			r.Left = append(r.Left, path)
			return nil
		case d.IsDir():
			return nil
		}
		r.Scanned++
		if !d.Type().IsRegular() || unmasked(root, path, mask) {
			r.Left = append(r.Left, path)
		}
		return nil
	})
	if err != nil {
		return Report{}, fmt.Errorf("rescan failed: %w", err)
	}
	slices.Sort(r.Left)
	r.Left = slices.Compact(r.Left)
	return r, nil
}

func unmasked(root *os.Root, path string, mask func(string) string) bool {
	f, err := openNoFollow(root, path)
	if err != nil {
		return true
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		return true
	}
	text := string(raw)
	return mask(text) != text
}

func openNoFollow(root *os.Root, rel string) (*os.File, error) {
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

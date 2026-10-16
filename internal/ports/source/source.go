package source

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/d0lim/cockpit/internal/ports"
)

type Roots map[string]string

type Reader struct {
	roots    map[string]*os.Root
	maxBytes int64
	mask     ports.Mask
}

func Open(roots Roots, maxBytes int64, mask ports.Mask) (*Reader, error) {
	r := &Reader{roots: map[string]*os.Root{}, maxBytes: maxBytes, mask: mask}
	for name, dir := range roots {
		root, err := os.OpenRoot(dir)
		if err != nil {
			r.Close()
			return nil, ports.Unavailable(mask, "failed to open source "+name)
		}
		r.roots[name] = root
	}
	return r, nil
}

func (r *Reader) Close() {
	for _, root := range r.roots {
		root.Close()
	}
}

func (r *Reader) locate(name, rel string) (*os.Root, error) {
	root, ok := r.roots[name]
	if !ok {
		return nil, ports.Denied(r.mask, "source "+name+" is not declared")
	}
	if !filepath.IsLocal(rel) {
		return nil, ports.Denied(r.mask, "path "+rel+" is outside source "+name)
	}
	return root, nil
}

func (r *Reader) Read(name, rel string) (string, error) {
	root, err := r.locate(name, rel)
	if err != nil {
		return "", err
	}
	info, err := root.Lstat(rel)
	if err != nil {
		return "", ports.Unavailable(r.mask, "source "+name+" has no "+rel)
	}
	if !info.Mode().IsRegular() {
		return "", ports.Denied(r.mask, "source "+name+": "+rel+" is not a regular file")
	}
	if info.Size() > r.maxBytes {
		return "", ports.Limit(r.mask, "source "+name+": "+rel+" exceeds the size limit")
	}
	f, err := root.OpenFile(rel, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", ports.Denied(r.mask, "source "+name+": failed to open "+rel)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, r.maxBytes+1))
	if err != nil {
		return "", ports.Unavailable(r.mask, "source "+name+": failed to read "+rel)
	}
	if int64(len(data)) > r.maxBytes {
		return "", ports.Limit(r.mask, "source "+name+": "+rel+" exceeds the size limit")
	}
	return string(data), nil
}

func (r *Reader) List(name, rel string) ([]string, error) {
	root, err := r.locate(name, rel)
	if err != nil {
		return nil, err
	}
	dir, err := root.Open(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, ports.Unavailable(r.mask, "source "+name+": failed to open "+rel)
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, ports.Unavailable(r.mask, "source "+name+": "+rel+" is not a directory")
	}
	names := []string{}
	for _, e := range entries {
		if e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

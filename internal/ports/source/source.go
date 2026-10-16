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
			return nil, ports.Unavailable(mask, "소스 "+name+" 을 열지 못했다")
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
		return nil, ports.Denied(r.mask, "선언에 없는 소스 "+name)
	}
	if !filepath.IsLocal(rel) {
		return nil, ports.Denied(r.mask, "소스 "+name+" 밖 경로 "+rel)
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
		return "", ports.Unavailable(r.mask, "소스 "+name+" 에 "+rel+" 이 없다")
	}
	if !info.Mode().IsRegular() {
		return "", ports.Denied(r.mask, "소스 "+name+" 의 "+rel+" 은 일반 파일이 아니다")
	}
	if info.Size() > r.maxBytes {
		return "", ports.Limit(r.mask, "소스 "+name+" 의 "+rel+" 이 크기 상한을 넘는다")
	}
	f, err := root.OpenFile(rel, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", ports.Denied(r.mask, "소스 "+name+" 의 "+rel+" 을 열지 못했다")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, r.maxBytes+1))
	if err != nil {
		return "", ports.Unavailable(r.mask, "소스 "+name+" 의 "+rel+" 을 읽지 못했다")
	}
	if int64(len(data)) > r.maxBytes {
		return "", ports.Limit(r.mask, "소스 "+name+" 의 "+rel+" 이 크기 상한을 넘는다")
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
		return nil, ports.Unavailable(r.mask, "소스 "+name+" 의 "+rel+" 을 열지 못했다")
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, ports.Unavailable(r.mask, "소스 "+name+" 의 "+rel+" 은 디렉토리가 아니다")
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

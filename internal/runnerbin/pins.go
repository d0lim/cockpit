package runnerbin

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type FilePins struct {
	Path string
}

func (f FilePins) load() (map[string]Image, error) {
	byTag := map[string]Image{}
	raw, err := os.ReadFile(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return byTag, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &byTag); err != nil {
		return nil, err
	}
	return byTag, nil
}

func (f FilePins) Pinned(tag string) (string, bool) {
	byTag, err := f.load()
	if err != nil {
		return "", false
	}
	img, ok := byTag[tag]
	return img.ID, ok
}

func (f FilePins) Pin(img Image) error {
	byTag, err := f.load()
	if err != nil {
		return err
	}
	byTag[img.Tag] = img
	data, err := json.MarshalIndent(byTag, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.Path), ".pins-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), f.Path)
}

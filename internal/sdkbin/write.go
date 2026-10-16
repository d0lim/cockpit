package sdkbin

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/d0lim/cockpit"
)

const sdkDir = "sdk"

func files() (fs.FS, error) {
	return fs.Sub(cockpit.SDK, sdkDir)
}

func Names() ([]string, error) {
	sub, err := files()
	if err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(sub, ".")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

func Write(dir string) ([]string, error) {
	sub, err := files()
	if err != nil {
		return nil, err
	}
	names, err := Names()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	for _, name := range names {
		data, err := fs.ReadFile(sub, name)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return nil, err
		}
	}
	return names, nil
}

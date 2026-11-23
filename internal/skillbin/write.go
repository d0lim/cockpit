package skillbin

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/d0lim/cockpit"
)

const (
	Name     = "cockpit"
	Entry    = "SKILL.md"
	skillDir = "skill/" + Name
)

func files() (fs.FS, error) {
	return fs.Sub(cockpit.Skill, skillDir)
}

func Names() ([]string, error) {
	sub, err := files()
	if err != nil {
		return nil, err
	}
	var names []string
	err = fs.WalkDir(sub, ".", func(name string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			names = append(names, name)
		}
		return err
	})
	slices.Sort(names)
	return names, err
}

func Read(name string) ([]byte, error) {
	sub, err := files()
	if err != nil {
		return nil, err
	}
	return fs.ReadFile(sub, name)
}

func Write(dir string) ([]string, error) {
	names, err := Names()
	if err != nil {
		return nil, err
	}
	written := make([]string, 0, len(names))
	for _, name := range names {
		data, err := Read(name)
		if err != nil {
			return nil, err
		}
		target := filepath.Join(dir, Name, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return nil, err
		}
		written = append(written, filepath.Join(Name, filepath.FromSlash(name)))
	}
	return written, nil
}

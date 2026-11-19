package report

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

var errKeptInPartial = errors.New("report: the kept directory could not be put back and stays in the partial bundle")

func clearFinal(final, keep string) error {
	if keep == "" {
		return os.RemoveAll(final)
	}
	entries, err := os.ReadDir(final)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	kept := slices.ContainsFunc(entries, func(e fs.DirEntry) bool { return e.Name() == keep && e.IsDir() })
	if !kept {
		return os.RemoveAll(final)
	}
	for _, e := range entries {
		if e.Name() == keep {
			continue
		}
		if err := os.RemoveAll(filepath.Join(final, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func publish(partial, final, keep string) error {
	held := filepath.Join(final, keep)
	if info, err := os.Lstat(held); keep == "" || err != nil || !info.IsDir() {
		return os.Rename(partial, final)
	}
	carried := filepath.Join(partial, keep)
	if err := os.Rename(held, carried); err != nil {
		return err
	}
	err := os.Remove(final)
	if err == nil {
		err = os.Rename(partial, final)
	}
	if err == nil {
		return nil
	}
	if back := errors.Join(os.MkdirAll(final, 0o700), os.Rename(carried, held)); back != nil {
		return errors.Join(errKeptInPartial, err, back)
	}
	return err
}

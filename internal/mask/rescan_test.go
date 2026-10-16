package mask

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

func rescanUnreadableScenario(e engine) []string {
	m, err := e.build(Options{Mode: Hub, Known: []string{planted}})
	if err != nil {
		return []string{err.Error()}
	}
	clean, _ := New(Options{Mode: Hub, Known: []string{planted}})
	dir, done, err := tree(map[string]string{
		"clean.txt":      "status 200",
		"masked.txt":     clean.Mask("value " + planted),
		"leaky.txt":      "value " + planted,
		"sub/locked.txt": "status 200",
	})
	defer done()
	if err != nil {
		return []string{err.Error()}
	}
	want := []string{"leaky.txt"}
	if os.Geteuid() != 0 {
		locked := filepath.Join(dir, "sub/locked.txt")
		if err := os.Chmod(locked, 0); err != nil {
			return []string{err.Error()}
		}
		defer os.Chmod(locked, 0o600)
		want = append(want, "sub/locked.txt")
	}
	r, err := e.rescan(dir, func(s string) string { return e.mask(m, s) })
	switch {
	case err != nil:
		return []string{err.Error()}
	case !slices.Equal(r.Left, want) || r.Scanned != 4:
		return []string{fmt.Sprintf("left files %q and scanned %d, want %q and 4", r.Left, r.Scanned, want)}
	}
	return nil
}

func rescanLinkScenario(e engine) []string {
	m, err := e.build(Options{Mode: Hub, Known: []string{planted}})
	if err != nil {
		return []string{err.Error()}
	}
	outside, doneOut, err := tree(map[string]string{"clean.txt": "status 200", "nested/inner.txt": "value " + planted})
	defer doneOut()
	if err != nil {
		return []string{err.Error()}
	}
	dir, done, err := tree(map[string]string{"ok.txt": "status 200"})
	defer done()
	if err != nil {
		return []string{err.Error()}
	}
	if err := os.Symlink(filepath.Join(outside, "clean.txt"), filepath.Join(dir, "file-link.txt")); err != nil {
		return []string{err.Error()}
	}
	if err := os.Symlink(filepath.Join(outside, "nested"), filepath.Join(dir, "dir-link")); err != nil {
		return []string{err.Error()}
	}
	r, err := e.rescan(dir, func(s string) string { return e.mask(m, s) })
	switch {
	case err != nil:
		return []string{err.Error()}
	case !slices.Equal(r.Left, []string{"dir-link", "file-link.txt"}) || r.Scanned != 3:
		return []string{fmt.Sprintf("left files %q and scanned %d, want [dir-link file-link.txt] and 3", r.Left, r.Scanned)}
	}
	return nil
}

func rescanMissingDirScenario(e engine) []string {
	m, err := e.build(Options{Mode: Hub})
	if err != nil {
		return []string{err.Error()}
	}
	dir, done, err := tree(nil)
	defer done()
	if err != nil {
		return []string{err.Error()}
	}
	if _, err := e.rescan(filepath.Join(dir, "absent"), m.Mask); err == nil {
		return []string{"rescanning a missing directory returned no error"}
	}
	return nil
}

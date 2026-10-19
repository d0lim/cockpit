package compose

import (
	"context"
	"fmt"
	"strings"
)

type Entry struct {
	Mode string
	Type string
	ID   string
}

type Tree struct {
	SHA     string
	Entries map[string]Entry
}

func (g Git) ListTree(ctx context.Context, sha string) (Tree, error) {
	if !shaPattern.MatchString(sha) {
		return Tree{}, fmt.Errorf("%w: tree listing needs a resolved sha, got %q", ErrCommitInput, sha)
	}
	out, err := g.Run(ctx, "ls-tree", "-r", "-t", "-z", "--full-tree", sha)
	if err != nil {
		return Tree{}, err
	}
	t := Tree{SHA: sha, Entries: map[string]Entry{}}
	for _, rec := range strings.Split(string(out), "\x00") {
		if rec == "" {
			continue
		}
		meta, path, ok := strings.Cut(rec, "\t")
		parts := strings.Fields(meta)
		if !ok || len(parts) != 3 {
			return Tree{}, fmt.Errorf("unexpected ls-tree record %q", rec)
		}
		t.Entries[path] = Entry{Mode: parts[0], Type: parts[1], ID: parts[2]}
	}
	return t, nil
}

func (t Tree) Link(path string) (string, bool) {
	parts := strings.Split(path, "/")
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		if e, ok := t.Entries[prefix]; ok && e.Mode == symlinkMode {
			return prefix, true
		}
	}
	return "", false
}

func (t Tree) Has(path string) bool {
	_, ok := t.Entries[path]
	return ok
}

func (t Tree) IsDir(path string) bool {
	return t.Entries[path].Type == treeType
}

func (t Tree) Files() int {
	n := 0
	for _, e := range t.Entries {
		if e.Type != treeType && e.Type != submoduleType {
			n++
		}
	}
	return n
}

func (t Tree) Under(dir string) int {
	n := 0
	for path, e := range t.Entries {
		if e.Type != treeType && e.Type != submoduleType && strings.HasPrefix(path, dir+"/") {
			n++
		}
	}
	return n
}

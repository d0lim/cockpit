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
	repo string
}

type Tree struct {
	SHA     string
	Entries map[string]Entry
	repos   map[string]repository
}

type repository struct {
	prefix  string
	commit  string
	objects objects
}

func (g Git) ListTree(ctx context.Context, sha string) (Tree, error) {
	if !shaPattern.MatchString(sha) {
		return Tree{}, fmt.Errorf("%w: tree listing needs a resolved sha, got %q", ErrCommitInput, sha)
	}
	common, err := g.CommonDir(ctx)
	if err != nil {
		return Tree{}, err
	}
	root := repository{commit: sha, objects: objects{dir: common}}
	listed, err := root.objects.list(ctx, sha)
	if err != nil {
		return Tree{}, err
	}
	return Tree{SHA: sha, Entries: listed, repos: map[string]repository{root.prefix: root}}, nil
}

func (t Tree) Blob(ctx context.Context, path string) ([]byte, error) {
	e, ok := t.Entries[path]
	if !ok {
		return nil, fmt.Errorf("%s is not in the commit", path)
	}
	return t.repos[e.repo].objects.blob(ctx, e.ID)
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

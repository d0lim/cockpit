package compose

import (
	"context"
	"fmt"
	"slices"
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
	parent  string
	name    string
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
	t := Tree{SHA: sha, Entries: map[string]Entry{}, repos: map[string]repository{}}
	if err := t.splice(ctx, repository{commit: sha, objects: objects{dir: common}}); err != nil {
		return Tree{}, err
	}
	return t, nil
}

func (t Tree) splice(ctx context.Context, r repository) error {
	listed, err := r.objects.list(ctx, r.commit)
	switch {
	case err != nil && r.prefix == "":
		return err
	case err != nil:
		return reject(RejectSubmodule, r.prefix, "submodule %q pins commit %s, which %s does not hold (%v); %s", r.name, r.commit, r.objects.dir, err, submoduleHint)
	}
	t.repos[r.prefix] = r
	var links []string
	for rel, e := range listed {
		e.repo = r.prefix
		t.Entries[join(r.prefix, rel)] = e
		if e.Type == submoduleType {
			links = append(links, rel)
		}
	}
	if len(links) == 0 {
		return nil
	}
	names, err := r.objects.submoduleNames(ctx, r.prefix, listed)
	if err != nil {
		return err
	}
	slices.Sort(links)
	for _, rel := range links {
		child, err := r.submodule(rel, listed[rel].ID, names, len(t.repos)-1)
		if err != nil {
			return err
		}
		link := t.Entries[child.prefix]
		link.repo = child.prefix
		t.Entries[child.prefix] = link
		if err := t.splice(ctx, child); err != nil {
			return err
		}
	}
	return nil
}

func join(prefix, rel string) string {
	if prefix == "" {
		return rel
	}
	return prefix + "/" + rel
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
	switch t.Entries[path].Type {
	case treeType, submoduleType:
		return true
	}
	return false
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

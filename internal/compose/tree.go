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

func (g Git) ListTree(ctx context.Context, sha string, paths ...string) (Tree, error) {
	if !shaPattern.MatchString(sha) {
		return Tree{}, fmt.Errorf("%w: tree listing needs a resolved sha, got %q", ErrCommitInput, sha)
	}
	common, err := g.CommonDir(ctx)
	if err != nil {
		return Tree{}, err
	}
	t := Tree{SHA: sha, Entries: map[string]Entry{}, repos: map[string]repository{}}
	if err := t.add(ctx, repository{commit: sha, objects: objects{dir: common}}); err != nil {
		return Tree{}, err
	}
	if err := t.splice(ctx, paths); err != nil {
		return Tree{}, err
	}
	return t, nil
}

func (t Tree) splice(ctx context.Context, paths []string) error {
	names := map[string]map[string]string{}
	for links := t.referenced(paths); len(links) > 0; links = t.referenced(paths) {
		for _, at := range links {
			if err := t.open(ctx, at, names); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t Tree) referenced(paths []string) []string {
	var links []string
	for at, e := range t.Entries {
		if t.isUnspliced(at, e) && slices.ContainsFunc(paths, func(p string) bool { return covers(at, p) || covers(p, at) }) {
			links = append(links, at)
		}
	}
	slices.Sort(links)
	return links
}

func (t Tree) spliced(at string) bool {
	_, ok := t.repos[at]
	return ok
}

func (t Tree) open(ctx context.Context, at string, names map[string]map[string]string) error {
	link := t.Entries[at]
	parent := t.repos[link.repo]
	known, ok := names[parent.prefix]
	if !ok {
		var err error
		if known, err = t.submoduleNames(ctx, parent); err != nil {
			return err
		}
		names[parent.prefix] = known
	}
	rel := at
	if parent.prefix != "" {
		rel = strings.TrimPrefix(at, parent.prefix+"/")
	}
	child, err := parent.submodule(rel, link.ID, known, len(t.repos)-1)
	if err != nil {
		return err
	}
	link.repo = child.prefix
	t.Entries[at] = link
	return t.add(ctx, child)
}

func (t Tree) add(ctx context.Context, r repository) error {
	listed, err := r.objects.list(ctx, r.commit)
	switch {
	case err != nil && r.prefix == "":
		return err
	case err != nil:
		return reject(RejectSubmodule, r.prefix, "submodule %q pins commit %s, which %s does not hold (%v); %s", r.name, r.commit, r.objects.dir, err, submoduleHint)
	}
	t.repos[r.prefix] = r
	for rel, e := range listed {
		e.repo = r.prefix
		t.Entries[join(r.prefix, rel)] = e
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
	return t.atOrAbove(path, func(_ string, e Entry) bool { return e.Mode == symlinkMode })
}

func (t Tree) isUnspliced(at string, e Entry) bool {
	return e.Type == submoduleType && !t.spliced(at)
}

func (t Tree) unspliced(path string) (string, bool) {
	return t.atOrAbove(path, t.isUnspliced)
}

func (t Tree) LeftOut(path string) []string {
	if link, ok := t.unspliced(path); ok {
		return []string{link}
	}
	var links []string
	for at, e := range t.Entries {
		if covers(path, at) && t.isUnspliced(at, e) {
			links = append(links, at)
		}
	}
	slices.Sort(links)
	return links
}

func (t Tree) atOrAbove(path string, match func(string, Entry) bool) (string, bool) {
	parts := strings.Split(path, "/")
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		if e, ok := t.Entries[prefix]; ok && match(prefix, e) {
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

func (t Tree) repositories() []repository {
	out := make([]repository, 0, len(t.repos))
	for _, prefix := range sortedKeys(t.repos) {
		out = append(out, t.repos[prefix])
	}
	return out
}

func (t Tree) filesOf(prefix string) int {
	n := 0
	for _, e := range t.Entries {
		if e.repo == prefix && e.Type != treeType && e.Type != submoduleType {
			n++
		}
	}
	return n
}

package compose

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type dirtyStep struct {
	prefix    string
	pathspecs []string
}

type checkout int

const (
	absent checkout = iota
	own
	foreign
)

func (g Git) Dirty(ctx context.Context, tree Tree, paths []string) ([]string, error) {
	top, err := g.TopLevel(ctx)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, step := range dirtyPlan(tree, paths) {
		dir := filepath.Join(top, filepath.FromSlash(step.prefix))
		if step.prefix != "" {
			switch checkoutState(ctx, dir) {
			case absent:
				continue
			case foreign:
				files = append(files, step.prefix)
				continue
			}
		}
		listed, err := Git{Dir: dir}.status(ctx, step.pathspecs)
		if err != nil {
			return nil, err
		}
		for _, f := range listed {
			files = append(files, join(step.prefix, f))
		}
	}
	slices.Sort(files)
	return slices.Compact(files), nil
}

func dirtyPlan(tree Tree, paths []string) []dirtyStep {
	set := map[string]bool{"": true}
	for prefix := range tree.repos {
		set[prefix] = true
	}
	prefixes := sortedKeys(set)
	specs := map[string][]string{}
	add := func(prefix, spec string) { specs[prefix] = append(specs[prefix], spec) }
	if len(paths) == 0 {
		for _, prefix := range prefixes {
			add(prefix, ".")
		}
	}
	for _, p := range paths {
		owner := ""
		for _, prefix := range prefixes[1:] {
			if covers(prefix, p) {
				owner = prefix
			}
			if covers(p, prefix) {
				add(prefix, ".")
			}
		}
		if p == owner {
			add(owner, ".")
		} else {
			add(owner, strings.TrimPrefix(p, owner+"/"))
		}
	}
	for i := len(prefixes) - 1; i > 0; i-- {
		if child := prefixes[i]; specs[child] != nil {
			parent := tree.repos[child].parent
			add(parent, strings.TrimPrefix(child, parent+"/"))
		}
	}
	var steps []dirtyStep
	for _, prefix := range prefixes {
		s := specs[prefix]
		switch {
		case s == nil:
			continue
		case slices.Contains(s, "."):
			s = []string{"."}
		default:
			slices.Sort(s)
			s = slices.Compact(s)
		}
		steps = append(steps, dirtyStep{prefix: prefix, pathspecs: s})
	}
	return steps
}

func covers(dir, p string) bool {
	return dir == "." || p == dir || strings.HasPrefix(p, dir+"/")
}

func checkoutState(ctx context.Context, dir string) checkout {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) || err == nil && len(entries) == 0 {
		return absent
	}
	real, realErr := filepath.EvalSymlinks(dir)
	top, topErr := Git{Dir: dir}.TopLevel(ctx)
	if err != nil || realErr != nil || topErr != nil || top != real {
		return foreign
	}
	return own
}

func (g Git) status(ctx context.Context, pathspecs []string) ([]string, error) {
	args := append([]string{"status", "--porcelain", "-z", "--untracked-files=normal", "--ignore-submodules=dirty", "--"}, pathspecs...)
	out, err := g.Run(ctx, args...)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(string(out), "\x00")
	var files []string
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 4 {
			continue
		}
		files = append(files, f[3:])
		if strings.ContainsAny(f[:2], "RC") {
			i++
		}
	}
	return files, nil
}

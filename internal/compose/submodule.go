package compose

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	gitmodulesPath = ".gitmodules"
	modulesDir     = "modules"
	maxSubmodules  = 64
	submoduleHint  = "run git submodule update --init --recursive in the main checkout; the hub does not read a linked worktree's submodules"
)

func (r repository) submodule(rel, commit string, names map[string]string, spliced int) (repository, error) {
	at := join(r.prefix, rel)
	if !cleanRelative(rel) {
		return repository{}, reject(RejectSubmodule, at, "the gitlink path is not a clean relative path")
	}
	if spliced >= maxSubmodules {
		return repository{}, reject(RejectSubmodule, at, "the commit pins more than %d submodules", maxSubmodules)
	}
	name, ok := names[rel]
	if !ok {
		return repository{}, reject(RejectSubmodule, at, "the gitlink has no submodule entry in %s", join(r.prefix, gitmodulesPath))
	}
	store, err := r.objects.module(name)
	if err != nil {
		return repository{}, reject(RejectSubmodule, at, "%v", err)
	}
	return repository{prefix: at, parent: r.prefix, name: name, commit: commit, objects: store}, nil
}

func (o objects) submoduleNames(ctx context.Context, prefix string, listed map[string]Entry) (map[string]string, error) {
	at := join(prefix, gitmodulesPath)
	e, ok := listed[gitmodulesPath]
	switch {
	case !ok:
		return map[string]string{}, nil
	case e.Type != "blob" || e.Mode == symlinkMode:
		return nil, reject(RejectSubmodule, at, "is not a regular file")
	}
	raw, err := o.config(ctx, e.ID)
	if err != nil {
		return nil, reject(RejectSubmodule, at, "is not readable git config: %v", err)
	}
	names, err := parseSubmoduleNames(raw)
	if err != nil {
		return nil, reject(RejectSubmodule, at, "%v", err)
	}
	return names, nil
}

func (o objects) config(ctx context.Context, blob string) ([]byte, error) {
	if !shaPattern.MatchString(blob) {
		return nil, fmt.Errorf("not an object id: %q", blob)
	}
	var buf bytes.Buffer
	w := &capWriter{buf: &buf, max: maxBlobBytes}
	if err := o.stream(ctx, w, "config", "-z", "--no-includes", "--blob", blob, "--list"); err != nil {
		return nil, err
	}
	if w.over {
		return nil, fmt.Errorf("the listing exceeds %d bytes", maxBlobBytes)
	}
	return buf.Bytes(), nil
}

func parseSubmoduleNames(listing []byte) (map[string]string, error) {
	names := map[string]string{}
	for _, rec := range strings.Split(string(listing), "\x00") {
		key, value, ok := strings.Cut(rec, "\n")
		rest, section := strings.CutPrefix(key, "submodule.")
		name, isPath := strings.CutSuffix(rest, ".path")
		if !ok || !section || !isPath || name == "" {
			continue
		}
		if other, taken := names[value]; taken && other != name {
			return nil, fmt.Errorf("path %q is claimed by submodules %q and %q", value, other, name)
		}
		names[value] = name
	}
	return names, nil
}

func cleanRelative(p string) bool {
	return p != "" && p != "." && !strings.ContainsAny(p, "\\\x00") && path.Clean(p) == p && filepath.IsLocal(filepath.FromSlash(p))
}

func (o objects) module(name string) (objects, error) {
	base := filepath.Join(o.dir, modulesDir)
	if !cleanRelative(name) {
		return objects{}, fmt.Errorf("submodule name %q must be a clean relative path inside %s", name, base)
	}
	root, err := os.OpenRoot(base)
	if err == nil {
		defer root.Close()
		_, err = root.Stat(filepath.FromSlash(name))
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return objects{}, fmt.Errorf("submodule %q is not initialized: %s does not exist; %s", name, filepath.Join(base, filepath.FromSlash(name)), submoduleHint)
	case err != nil:
		return objects{}, fmt.Errorf("submodule name %q resolves outside %s: %v", name, base, err)
	}
	dir, err := filepath.EvalSymlinks(filepath.Join(base, filepath.FromSlash(name)))
	if err != nil {
		return objects{}, err
	}
	return objects{dir: dir}, nil
}

package compose

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/d0lim/cockpit/internal/contract"
)

const maxComposeReads = 64

type composeFile struct {
	path string
	base string
}

type includeKind int

const (
	includePath includeKind = iota
	includeProjectDirectory
	includeEnvFile
	includeDotenv
)

type includeValue struct {
	field string
	kind  includeKind
	value string
}

type includeItem struct {
	field    string
	paths    []includeValue
	project  []includeValue
	envFiles []includeValue
}

type walk struct {
	ctx       context.Context
	tree      Tree
	outputs   []contract.RelPath
	forbidden []string
	model     *Model
	chain     map[composeFile]bool
	reads     int
}

func (w *walk) visit(field string, f composeFile) error {
	if w.chain[f] {
		return reject(RejectCompose, field, "include cycle at %s", f.path)
	}
	if w.reads == maxComposeReads {
		return reject(RejectCompose, field, "the compose files and their includes read more than %d files", maxComposeReads)
	}
	w.reads++
	raw, err := w.tree.Blob(w.ctx, f.path)
	if err != nil {
		return reject(RejectCompose, field, "cannot read %s: %v", f.path, err)
	}
	root, err := parseRoot(raw)
	if err != nil {
		return reject(RejectCompose, field, "%s: %v", f.path, err)
	}
	w.model.read(f.path)
	items, err := includeItems(field, f.path, root)
	if err != nil {
		return err
	}
	w.chain[f] = true
	defer delete(w.chain, f)
	for _, item := range items {
		if err := w.include(f, item); err != nil {
			return err
		}
	}
	w.model.add(root)
	return nil
}

func (w *walk) include(from composeFile, item includeItem) error {
	paths := make([]string, 0, len(item.paths))
	for _, v := range item.paths {
		p, r := w.confine(from.path, from.base, v)
		if r != nil {
			return r
		}
		paths = append(paths, p)
	}
	base := dirOf(paths[0])
	for _, v := range item.project {
		dir, r := w.confine(from.path, from.base, v)
		if r != nil {
			return r
		}
		base = dir
	}
	for _, v := range item.envFiles {
		if _, r := w.confine(from.path, from.base, v); r != nil {
			return r
		}
	}
	if len(item.envFiles) == 0 {
		if _, r := w.confine(from.path, base, includeValue{field: item.field, kind: includeDotenv, value: dotenvName}); r != nil {
			return r
		}
	}
	for i, p := range paths {
		if err := w.visit(item.paths[i].field, composeFile{path: p, base: base}); err != nil {
			return err
		}
	}
	return nil
}

func (w *walk) confine(holder, base string, v includeValue) (string, *Rejection) {
	refuse := func(format string, args ...any) (string, *Rejection) {
		return "", reject(RejectCompose, v.field, "%s: "+format, append([]any{holder}, args...)...)
	}
	switch {
	case v.value == "":
		return refuse("the include value is empty")
	case strings.Contains(v.value, "$"):
		return refuse("%q uses interpolation, which the hub cannot resolve", v.value)
	case strings.Contains(v.value, ":"):
		return refuse("%q holds a colon, which marks a remote include (oci://, https://, git@) that compose fetches on the host", v.value)
	case strings.HasPrefix(v.value, "~"):
		return refuse("%q starts with ~, which compose expands to the home directory of the hub user", v.value)
	case path.IsAbs(v.value):
		return refuse("%q is an absolute path", v.value)
	}
	resolved := path.Join(base, v.value)
	if resolved == "." {
		resolved = ""
	}
	if resolved != "" && !cleanRelative(resolved) {
		return refuse("%q resolves to %q, outside the commit", v.value, resolved)
	}
	if link, ok := w.tree.Link(resolved); ok {
		return refuse("%q passes through the symbolic link %s in the commit", v.value, link)
	}
	switch v.kind {
	case includePath:
		if !w.file(resolved) {
			return refuse("%q resolves to %q, which is not a file in the commit", v.value, resolved)
		}
		if AssertSealedFiles([]string{resolved}, w.forbidden) != nil {
			return refuse("%q resolves to %q, whose name contains an entry of compose.forbidden", v.value, resolved)
		}
	case includeProjectDirectory:
		if resolved != "" && !w.tree.IsDir(resolved) {
			return refuse("%q resolves to %q, which is not a directory in the commit", v.value, resolved)
		}
	case includeEnvFile:
		if !w.file(resolved) && !slices.Contains(w.outputs, contract.RelPath(resolved)) {
			return refuse("%q resolves to %q, which is neither a file in the commit nor one of env.outputs", v.value, resolved)
		}
	}
	return resolved, nil
}

func (w *walk) file(p string) bool {
	e, ok := w.tree.Entries[p]
	return ok && e.Type == "blob"
}

func dirOf(p string) string {
	if dir := path.Dir(p); dir != "." {
		return dir
	}
	return ""
}

func includeItems(field, holder string, root *yaml.Node) ([]includeItem, error) {
	list := lookup(root, "include")
	if list == nil {
		return nil, nil
	}
	if list.Kind != yaml.SequenceNode {
		return nil, reject(RejectCompose, field+" include", "%s: include is not a list", holder)
	}
	items := make([]includeItem, 0, len(list.Content))
	for i, raw := range list.Content {
		item, err := parseItem(fmt.Sprintf("%s include[%d]", field, i), holder, deref(raw))
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func parseItem(at, holder string, n *yaml.Node) (includeItem, error) {
	item := includeItem{field: at}
	switch n.Kind {
	case yaml.ScalarNode:
		item.paths = []includeValue{{field: at, kind: includePath, value: n.Value}}
		return item, nil
	case yaml.MappingNode:
	default:
		return includeItem{}, reject(RejectCompose, at, "%s: an include item is a path or a mapping of path, project_directory and env_file", holder)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, value := n.Content[i].Value, deref(n.Content[i+1])
		field := at + "." + key
		var err error
		switch key {
		case "path":
			item.paths, err = includeValues(field, holder, includePath, value, true)
		case "project_directory":
			item.project, err = includeValues(field, holder, includeProjectDirectory, value, false)
		case "env_file":
			item.envFiles, err = includeValues(field, holder, includeEnvFile, value, true)
		default:
			err = reject(RejectCompose, field, "%s: %q is not an include key; an include item holds path, project_directory and env_file", holder, key)
		}
		if err != nil {
			return includeItem{}, err
		}
	}
	if len(item.paths) == 0 {
		return includeItem{}, reject(RejectCompose, at, "%s: the include item names no path", holder)
	}
	return item, nil
}

func includeValues(field, holder string, kind includeKind, n *yaml.Node, list bool) ([]includeValue, error) {
	if n.Kind == yaml.ScalarNode {
		return []includeValue{{field: field, kind: kind, value: n.Value}}, nil
	}
	if !list || n.Kind != yaml.SequenceNode {
		return nil, reject(RejectCompose, field, "%s: the value is not a string or, for path and env_file, a list of strings", holder)
	}
	out := make([]includeValue, 0, len(n.Content))
	for i, e := range n.Content {
		at := fmt.Sprintf("%s[%d]", field, i)
		if e = deref(e); e.Kind != yaml.ScalarNode {
			return nil, reject(RejectCompose, at, "%s: the value is not a string", holder)
		}
		out = append(out, includeValue{field: at, kind: kind, value: e.Value})
	}
	return out, nil
}

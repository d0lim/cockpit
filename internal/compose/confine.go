package compose

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
)

var namePattern = regexp.MustCompile(contract.NamePattern)

type declaredPath struct {
	field    string
	path     string
	mustDir  bool
	mustFile bool
}

type declaredName struct {
	field string
	name  string
	group string
}

func declaredPaths(d contract.Declaration) []declaredPath {
	var out []declaredPath
	add := func(field, path string, dir, file bool) {
		out = append(out, declaredPath{field: field, path: path, mustDir: dir, mustFile: file})
	}
	for i, p := range d.Compose.Files {
		add(fmt.Sprintf("compose.files[%d]", i), string(p), false, true)
	}
	if d.Compose.ProjectDirectory != "" {
		add("compose.projectDirectory", string(d.Compose.ProjectDirectory), true, false)
	}
	for i, p := range d.Inputs.Dirty {
		add(fmt.Sprintf("inputs.dirty[%d]", i), string(p), false, false)
	}
	for i, s := range d.Inputs.RunnerSources {
		add(fmt.Sprintf("inputs.runnerSources[%d].path", i), string(s.Path), true, false)
	}
	for i, img := range d.Images {
		for j, p := range img.Inputs {
			add(fmt.Sprintf("images[%d].inputs[%d]", i, j), string(p), false, false)
		}
	}
	for i, p := range d.Env.Outputs {
		add(fmt.Sprintf("env.outputs[%d]", i), string(p), false, false)
	}
	for i, s := range d.Secrets.Sources {
		add(fmt.Sprintf("secrets.sources[%d].path", i), string(s.Path), false, false)
	}
	add("scripts.dir", string(d.Scripts.Dir), true, false)
	add("scripts.suites", string(d.Scripts.Suites), true, false)
	if d.Scripts.Collectors != "" {
		add("scripts.collectors", string(d.Scripts.Collectors), true, false)
	}
	return out
}

func declaredNames(d contract.Declaration) []declaredName {
	var out []declaredName
	for i, p := range d.Ports {
		out = append(out, declaredName{fmt.Sprintf("ports[%d].name", i), string(p.Name), "ports"})
	}
	for i, s := range d.Inputs.RunnerSources {
		out = append(out, declaredName{fmt.Sprintf("inputs.runnerSources[%d].name", i), string(s.Name), "runnerSources"})
	}
	for i, db := range d.Databases {
		out = append(out, declaredName{fmt.Sprintf("databases[%d].name", i), string(db.Name), "databases"})
	}
	return out
}

func confinePath(p declaredPath, tree Tree) *Rejection {
	if p.path == "" || !filepath.IsLocal(p.path) || filepath.ToSlash(filepath.Clean(p.path)) != p.path || strings.Contains(p.path, `\`) {
		return reject(RejectConfinement, p.field, "%q must be a clean relative path inside the commit", p.path)
	}
	if link, ok := tree.Link(p.path); ok {
		return reject(RejectConfinement, p.field, "%q passes through the symbolic link %s in the commit", p.path, link)
	}
	switch {
	case p.mustDir && !tree.IsDir(p.path):
		return reject(RejectConfinement, p.field, "%q is not a directory in the commit", p.path)
	case p.mustFile && (!tree.Has(p.path) || tree.IsDir(p.path)):
		return reject(RejectConfinement, p.field, "%q is not a file in the commit", p.path)
	}
	return nil
}

func confine(d contract.Declaration, tree Tree) error {
	for _, p := range declaredPaths(d) {
		if r := confinePath(p, tree); r != nil {
			return r
		}
	}
	for _, field := range []struct {
		name string
		path contract.RelPath
	}{{"scripts.suites", d.Scripts.Suites}, {"scripts.collectors", d.Scripts.Collectors}} {
		if field.path != "" && !strings.HasPrefix(string(field.path), string(d.Scripts.Dir)+"/") {
			return reject(RejectConfinement, field.name, "%q must be inside scripts.dir %q", field.path, d.Scripts.Dir)
		}
	}
	seen := map[string]string{}
	for _, n := range declaredNames(d) {
		if !namePattern.MatchString(n.name) {
			return reject(RejectConfinement, n.field, "%q must match %s", n.name, contract.NamePattern)
		}
		key := n.group + "/" + n.name
		if first, dup := seen[key]; dup {
			return reject(RejectConfinement, n.field, "%q is already used at %s", n.name, first)
		}
		seen[key] = n.field
	}
	return nil
}

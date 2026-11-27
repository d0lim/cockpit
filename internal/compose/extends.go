package compose

import (
	"path"
	"strings"

	"go.yaml.in/yaml/v3"
)

func (w *walk) extends(field string, f composeFile, root *yaml.Node) *Rejection {
	services := pairs(lookup(root, "services"))
	for _, name := range sortedKeys(services) {
		file := lookup(lookup(services[name], "extends"), "file")
		if file == nil || file.Kind != yaml.ScalarNode || strings.ContainsAny(file.Value, "$:~") || path.IsAbs(file.Value) {
			continue
		}
		resolved := path.Join(f.base, file.Value)
		if gitlink, ok := w.tree.unspliced(resolved); ok {
			return reject(RejectCompose, field+" services."+name+".extends.file", "%s: %q resolves to %q in the submodule %s, which no declared path references, so the run leaves it out", f.path, file.Value, resolved, gitlink)
		}
	}
	return nil
}

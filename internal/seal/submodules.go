package seal

import (
	"fmt"
	"path/filepath"
)

func leftOut(in Input, subject, path string) []string {
	resolved, ok := in.Realpath(path)
	if !ok {
		return nil
	}
	var out []string
	for _, root := range []string{in.Layout.Src, in.Layout.Build} {
		real, ok := in.Realpath(root)
		if !ok || !inside(real, resolved) {
			continue
		}
		rel, _ := filepath.Rel(real, resolved)
		for _, link := range in.Tree.LeftOut(filepath.ToSlash(rel)) {
			out = append(out, fmt.Sprintf("%s reaches the submodule %s, which no declared path references, so the run leaves it out", subject, link))
		}
	}
	return out
}

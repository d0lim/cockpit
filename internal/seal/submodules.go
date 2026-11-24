package seal

import (
	"fmt"
	"path/filepath"
	"slices"
)

func leftOut(in Input, subject, path string) []string {
	resolved, ok := in.Realpath(path)
	if !ok {
		return nil
	}
	var links []string
	for _, root := range []string{in.Layout.Src, in.Layout.Build} {
		real, ok := in.Realpath(root)
		switch {
		case !ok:
		case inside(real, resolved):
			rel, _ := filepath.Rel(real, resolved)
			links = append(links, in.Tree.LeftOut(filepath.ToSlash(rel))...)
		case inside(resolved, real):
			links = append(links, in.Tree.LeftOut(".")...)
		}
	}
	slices.Sort(links)
	var out []string
	for _, link := range slices.Compact(links) {
		out = append(out, fmt.Sprintf("%s reaches the submodule %s, which no declared path references, so the run leaves it out", subject, link))
	}
	return out
}

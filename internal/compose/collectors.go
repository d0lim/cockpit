package compose

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
)

var ReservedCollectorIDs = []string{"runner", "logs"}

func CollectorIDs(decl contract.Declaration, tree Tree) []string {
	dir := string(decl.Scripts.Collectors)
	if dir == "" {
		return nil
	}
	var ids []string
	for p, e := range tree.Entries {
		if e.Type == "blob" && path.Dir(p) == dir && strings.HasSuffix(p, ".ts") {
			ids = append(ids, strings.TrimSuffix(path.Base(p), ".ts"))
		}
	}
	slices.Sort(ids)
	return ids
}

func checkCollectors(decl contract.Declaration, tree Tree) error {
	for _, id := range CollectorIDs(decl, tree) {
		field := string(decl.Scripts.Collectors) + "/" + id + ".ts"
		if !namePattern.MatchString(id) {
			return reject(RejectConfinement, field, "collector id %q must match %s", id, contract.NamePattern)
		}
		if slices.Contains(ReservedCollectorIDs, id) {
			return reject(RejectConfinement, field, "collector id %q is reserved for the hub (%s)", id, strings.Join(ReservedCollectorIDs, ", "))
		}
	}
	return nil
}

func SecretFilePatterns(decl contract.Declaration) []*regexp.Regexp {
	seen := map[string]bool{}
	var out []*regexp.Regexp
	add := func(p contract.RelPath) {
		base := path.Base(string(p))
		if base == "" || base == "." || seen[base] {
			return
		}
		seen[base] = true
		out = append(out, regexp.MustCompile(`(?:^|/)`+regexp.QuoteMeta(base)+`$`))
	}
	for _, s := range decl.Secrets.Sources {
		add(s.Path)
	}
	for _, o := range decl.Env.Outputs {
		add(o)
	}
	return out
}

func Suites(decl contract.Declaration, tree Tree) []string {
	dir := string(decl.Scripts.Suites)
	var names []string
	for p, e := range tree.Entries {
		if e.Type == "blob" && path.Dir(p) == dir && strings.HasSuffix(p, ".ts") {
			names = append(names, strings.TrimSuffix(path.Base(p), ".ts"))
		}
	}
	slices.Sort(names)
	return names
}

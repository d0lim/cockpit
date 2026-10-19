package seal

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
)

type Result struct {
	Name         string
	OK           bool
	Inconclusive bool
	Command      string
	Expected     string
	Detail       string
}

type Input struct {
	Config      map[string]any
	Files       []string
	Forbidden   []string
	RunID       string
	RunDir      string
	Layout      compose.Layout
	PortBase    int
	Ports       []contract.PortOffset
	Builder     string
	RunnerImage string
	Realpath    func(path string) (string, bool)
}

type doc = map[string]any

type service struct {
	name string
	doc  doc
}

func record(v any) doc {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return doc{}
}

func list(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return nil
}

func text(v any) string {
	s, _ := v.(string)
	return s
}

func present(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}

func result(name, expected string, violations []string) Result {
	r := Result{Name: name, OK: len(violations) == 0, Expected: expected, Detail: "no violation"}
	if !r.OK {
		r.Detail = strings.Join(violations, "; ")
	}
	return r
}

func services(config doc) []service {
	m := record(config["services"])
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]service, len(names))
	for i, name := range names {
		out[i] = service{name: name, doc: record(m[name])}
	}
	return out
}

func using(svcs []service, field string, allow ...string) []string {
	var out []string
	for _, s := range svcs {
		if !slices.Contains(allow, s.name) && present(s.doc[field]) {
			out = append(out, s.name)
		}
	}
	return out
}

func Static(in Input) []Result {
	if in.Realpath == nil {
		in.Realpath = RealPath
	}
	svcs := services(in.Config)
	out := []Result{roles(in)}
	out = append(out, result("files.overlays", "the file set holds no compose file matching compose.forbidden", overlays(in)))
	out = append(out, networkChecks(in.Config, svcs, in.Builder)...)
	out = append(out,
		result("services.ports", "host ports (ports) appear only on the edge", using(svcs, "ports", compose.RoleEdge)),
		result("services.network-mode", "network_mode appears nowhere", using(svcs, "network_mode")),
		result("services.extra-hosts", "extra_hosts appears nowhere", using(svcs, "extra_hosts")),
	)
	out = append(out, edgeChecks(in, svcs)...)
	out = append(out, mountChecks(in, svcs)...)
	out = append(out, volumeCheck(in.Config, svcs, in.Builder))
	out = append(out, privilegeChecks(svcs)...)
	out = append(out, pathChecks(in, svcs)...)
	return out
}

func roles(in Input) Result {
	expected := "exactly one service holds each harness role and every service carries the run labels"
	data, err := json.Marshal(in.Config)
	var r compose.Rendered
	if err == nil {
		err = json.Unmarshal(data, &r)
	}
	if err == nil {
		err = compose.AssertRoles(r, in.RunID, in.Builder)
	}
	if err != nil {
		return result("roles.present", expected, []string{err.Error()})
	}
	return result("roles.present", expected, nil)
}

func overlays(in Input) []string {
	var hits []string
	for _, f := range in.Files {
		if compose.AssertSealedFiles([]string{f}, in.Forbidden) != nil {
			hits = append(hits, f)
		}
	}
	return hits
}

func RealPath(path string) (string, bool) {
	if !filepath.IsAbs(path) {
		return "", false
	}
	var rest []string
	current := filepath.Clean(path)
	for {
		real, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(append([]string{real}, rest...)...), true
		}
		parent := filepath.Dir(current)
		if !errors.Is(err, fs.ErrNotExist) || parent == current {
			return "", false
		}
		if info, statErr := os.Lstat(current); statErr == nil && info.Mode()&fs.ModeSymlink != 0 {
			return "", false
		}
		rest = append([]string{filepath.Base(current)}, rest...)
		current = parent
	}
}

func inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}

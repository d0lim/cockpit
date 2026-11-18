package runner

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
)

var wirePlaceholder = regexp.MustCompile(`\$\{(COCKPIT_WIRE_[A-Z0-9_]+):-([^}]*)\}`)

type wireRoutes struct {
	proxyByUpstream map[string]string
	proxyByVariable map[string]string
}

func parseWireRoutes(value string) (wireRoutes, error) {
	routes := wireRoutes{proxyByUpstream: map[string]string{}, proxyByVariable: map[string]string{}}
	if value == "" {
		return routes, nil
	}
	for entry := range strings.SplitSeq(value, ",") {
		parts := strings.Split(entry, "=")
		if len(parts) != 3 || parts[0] == "" || !hostPort(parts[1]) || !hostPort(parts[2]) {
			return wireRoutes{}, fmt.Errorf("%s holds an entry that is not <name>=<service>:<port>=<proxy>:<port>", contract.WireRoutesVariable)
		}
		routes.proxyByUpstream[strings.ToLower(parts[1])] = parts[2]
		routes.proxyByVariable[contract.WireVariable(parts[0])] = parts[2]
	}
	return routes, nil
}

func hostPort(s string) bool {
	host, port, ok := strings.Cut(s, ":")
	return ok && host != "" && port != ""
}

func (w wireRoutes) recording() bool { return len(w.proxyByUpstream) > 0 }

func (w wireRoutes) rerouter() func(addr string) string {
	if !w.recording() {
		return nil
	}
	return func(addr string) string {
		if proxy, ok := w.proxyByUpstream[strings.ToLower(addr)]; ok {
			return proxy
		}
		return addr
	}
}

func (w wireRoutes) expandEnv(env map[string]any) map[string]any {
	out := make(map[string]any, len(env))
	for k, v := range env {
		out[k] = w.expandValue(v)
	}
	return out
}

func (w wireRoutes) expandValue(v any) any {
	switch x := v.(type) {
	case string:
		return wirePlaceholder.ReplaceAllStringFunc(x, func(found string) string {
			m := wirePlaceholder.FindStringSubmatch(found)
			if proxy, ok := w.proxyByVariable[m[1]]; ok {
				return proxy
			}
			return m[2]
		})
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = w.expandValue(e)
		}
		return out
	case map[string]any:
		return w.expandEnv(x)
	}
	return v
}

type wireValues struct {
	dir       string
	recording bool
}

func (w wireValues) update(known []string) error {
	if !w.recording {
		return nil
	}
	path := filepath.Join(w.dir, contract.WireValuesFile)
	values := slices.DeleteFunc(slices.Clone(known), func(v string) bool { return v == "" })
	if err := writeJSONFile(path, contract.WireValues{Values: values}, 0o600); err != nil {
		if rerr := os.RemoveAll(path); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			return fmt.Errorf("could not write or remove %s", contract.WireValuesFile)
		}
		return fmt.Errorf("could not write %s, so it was removed and the wire record of this run will be withheld", contract.WireValuesFile)
	}
	return nil
}

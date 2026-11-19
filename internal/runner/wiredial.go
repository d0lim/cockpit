package runner

import (
	"encoding/json"
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

var errWireValuesOverLimit = errors.New("the wire values file is over its limit")

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
	withheld  bool
	limit     int
	earlier   []string
}

func loadWireValues(dir string, recording bool) (*wireValues, error) {
	w := &wireValues{dir: dir, recording: recording, limit: contract.WireValuesLimit}
	if !recording {
		return w, nil
	}
	path := filepath.Join(dir, contract.WireValuesFile)
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		_, started := os.Lstat(filepath.Join(dir, fileManifest))
		w.withheld = !errors.Is(started, fs.ErrNotExist)
		return w, nil
	}
	var doc contract.WireValues
	if err == nil {
		err = json.Unmarshal(raw, &doc)
	}
	if err != nil {
		if rerr := os.RemoveAll(path); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			return w, fmt.Errorf("could not read or remove %s", contract.WireValuesFile)
		}
		return w, fmt.Errorf("could not read %s, so it was removed and the wire record of this run will be withheld", contract.WireValuesFile)
	}
	w.earlier = doc.Values
	return w, nil
}

func (w *wireValues) update(known []string) error {
	if !w.recording || w.withheld {
		return nil
	}
	path := filepath.Join(w.dir, contract.WireValuesFile)
	values := slices.Concat(known, w.earlier)
	slices.Sort(values)
	values = slices.DeleteFunc(slices.Compact(values), func(v string) bool { return v == "" })
	data, err := encodeJSONFile(contract.WireValues{Values: values})
	if err == nil && len(data) > w.limit {
		if rerr := os.RemoveAll(path); rerr != nil {
			return fmt.Errorf("could not remove %s", contract.WireValuesFile)
		}
		w.withheld = true
		return fmt.Errorf("%w: %d values take %d bytes and the limit is %d bytes, so it was removed and the wire record of this run will be withheld", errWireValuesOverLimit, len(values), len(data), w.limit)
	}
	if err == nil {
		err = writeFileAtomic(path, data, 0o600)
	}
	if err != nil {
		if rerr := os.RemoveAll(path); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			return fmt.Errorf("could not write or remove %s", contract.WireValuesFile)
		}
		return fmt.Errorf("could not write %s, so it was removed and the wire record of this run will be withheld", contract.WireValuesFile)
	}
	return nil
}

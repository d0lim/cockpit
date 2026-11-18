package fakedocker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/wire"
)

const WireContainerID = "fedcba9876543210fedc"

type WireRecorder struct {
	Unrendered bool
	Calls      []wire.Call
	Values     []string
	NoValues   bool
	Gone       bool
	Refusal    string
	Broken     int
	Since      time.Time
	Omitted    int
	Dropped    int

	mu       sync.Mutex
	rendered bool
	armed    []string
	taken    []string
}

func (r *WireRecorder) Armed() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.armed)
}

func (r *WireRecorder) Taken() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.taken)
}

func (f *Fake) Run(ctx context.Context, cmd compose.Command) (compose.Result, error) {
	res, err := f.run(ctx, cmd)
	if err != nil || res.Code != 0 {
		return res, err
	}
	if slices.Contains(cmd.Args, "wire-ctl") {
		return f.Wire.control(cmd), nil
	}
	switch Verb(cmd.Args) {
	case "compose config":
		if !f.Wire.Unrendered {
			return f.Wire.withRendered(cmd.Args, res), nil
		}
	case "compose ps":
		return f.Wire.withRow(res), nil
	case "compose run " + compose.RoleRunner:
		f.Wire.writeValues(cmd.Args)
	}
	return res, nil
}

func harnessFile(args []string) string {
	for i, a := range slices.Backward(args) {
		if i > 0 && args[i-1] == "-f" && filepath.Base(a) == compose.HarnessFile {
			return a
		}
	}
	return ""
}

func (r *WireRecorder) withRendered(args []string, res compose.Result) compose.Result {
	data, err := os.ReadFile(harnessFile(args))
	if err != nil {
		return res
	}
	var harness struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if yaml.Unmarshal(data, &harness) != nil {
		return res
	}
	recorder, ok := harness.Services[compose.RoleWire]
	if !ok {
		return res
	}
	networks := map[string]any{}
	listed, _ := recorder["networks"].([]any)
	for _, name := range listed {
		networks[name.(string)] = nil
	}
	recorder["networks"] = networks
	var config map[string]any
	if json.Unmarshal(res.Stdout, &config) != nil {
		return res
	}
	config["services"].(map[string]any)[compose.RoleWire] = recorder
	if res.Stdout, err = json.Marshal(config); err != nil {
		return compose.Result{Code: 1, Stderr: []byte(err.Error())}
	}
	r.mu.Lock()
	r.rendered = true
	r.mu.Unlock()
	return res
}

func (r *WireRecorder) withRow(res compose.Result) compose.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	var rows []map[string]any
	if !r.rendered || r.Gone || json.Unmarshal(res.Stdout, &rows) != nil {
		return res
	}
	rows = append(rows, map[string]any{"Service": compose.RoleWire, "ID": WireContainerID, "State": "running", "Health": "healthy"})
	res.Stdout, _ = json.Marshal(rows)
	return res
}

func (r *WireRecorder) writeValues(args []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.rendered {
		return
	}
	path := filepath.Join(filepath.Dir(flag(args, "--project-directory")), "work", "e2e", contract.WireValuesFile)
	if r.NoValues {
		os.Remove(path)
		return
	}
	write(path, contract.WireValues{Values: r.Values})
}

func (r *WireRecorder) control(cmd compose.Command) compose.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	verb, take := cmd.Args[len(cmd.Args)-2], cmd.Args[len(cmd.Args)-1]
	if r.Gone {
		return compose.Result{Code: 1, Stderr: []byte("the wire recorder is not reachable")}
	}
	if r.Refusal != "" {
		return compose.Result{Code: 1, Stderr: []byte(r.Refusal)}
	}
	if verb == "arm" {
		r.armed = append(r.armed, take)
		return compose.Result{}
	}
	r.taken = append(r.taken, take)
	for _, c := range r.Calls {
		line, _ := json.Marshal(wire.Line{Call: &c})
		fmt.Fprintf(cmd.Stdout, "%s\n", line)
		if r.Broken > 0 {
			break
		}
	}
	if r.Broken > 0 {
		r.Broken--
		return compose.Result{Code: 1, Stderr: []byte("the wire take broke off")}
	}
	end, _ := json.Marshal(wire.Line{End: &wire.End{Calls: len(r.Calls), BodiesOmitted: r.Omitted, CallsDropped: r.Dropped, Since: r.Since}})
	fmt.Fprintf(cmd.Stdout, "%s\n", end)
	return compose.Result{}
}

package fakedocker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/d0lim/cockpit/internal/compose"
)

type WireRecorder struct {
	Unrendered bool
}

func (f *Fake) Run(ctx context.Context, cmd compose.Command) (compose.Result, error) {
	res, err := f.run(ctx, cmd)
	if err != nil || res.Code != 0 {
		return res, err
	}
	if Verb(cmd.Args) == "compose config" && !f.Wire.Unrendered {
		return withRenderedRecorder(cmd.Args, res), nil
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

func withRenderedRecorder(args []string, res compose.Result) compose.Result {
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
	return res
}

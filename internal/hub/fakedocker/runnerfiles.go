package fakedocker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
)

const startedAt = "2026-10-19T01:00:00.000Z"

func (f *Fake) manifest() contract.Manifest {
	image := f.ManifestImage
	if image == "" {
		image = RunnerTag
	}
	m := contract.Manifest{SchemaVersion: contract.RunnerSchemaVersion, Suite: "smoke", Stages: []contract.ManifestStage{}, Flows: []contract.ManifestFlow{},
		Runner: &contract.RunnerIdentity{Version: f.Version, Image: image}}
	for _, flow := range f.Flows {
		if !slices.ContainsFunc(m.Stages, func(s contract.ManifestStage) bool { return s.ID == flow.Stage }) {
			m.Stages = append(m.Stages, contract.ManifestStage{ID: flow.Stage, Title: flow.Stage})
		}
		flow.Title, flow.Steps = flow.ID, []contract.ManifestStep{}
		for _, s := range f.Steps {
			if s.Flow == flow.ID {
				flow.Steps = append(flow.Steps, contract.ManifestStep{ID: s.ID, Title: s.ID, Kind: contract.StepKindRequest})
			}
		}
		m.Flows = append(m.Flows, flow)
	}
	return m
}

func (f *Fake) result() contract.Result {
	return contract.Result{SchemaVersion: contract.RunnerSchemaVersion, Suite: "smoke", Complete: !f.Incomplete, StartedAt: startedAt, FinishedAt: startedAt, StoppedAt: f.StoppedAt, Steps: f.Steps}
}

func (f *Fake) writeRunnerFiles(args, env []string) {
	src := flag(args, "--project-directory")
	e2e := filepath.Join(filepath.Dir(src), "work", "e2e")
	if f.OnRunner != nil {
		f.OnRunner(filepath.Dir(src))
	}
	write(filepath.Join(e2e, "manifest.json"), f.manifest())
	write(filepath.Join(e2e, "result.json"), f.result())
	for _, kv := range env {
		if name, ok := strings.CutPrefix(kv, "E2E_EXEC_OUT=/work/e2e/"); ok {
			write(filepath.Join(e2e, name), contract.ExecResult{Result: f.result(), Target: flag(args, "--flow")})
		}
	}
}

func write(path string, v any) {
	data, _ := json.Marshal(v)
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, data, 0o600)
}

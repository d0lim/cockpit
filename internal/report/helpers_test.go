package report

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
)

const planted = "PLANTED-bundle-secret-5c1e9b"

const declaredLine = "Later — the outbound leg is not wired"

func manifestFixture() *contract.Manifest {
	return &contract.Manifest{
		SchemaVersion: contract.RunnerSchemaVersion,
		Suite:         "toy",
		Stages:        []contract.ManifestStage{{ID: "basic", Title: "Basic"}, {ID: "later", Title: "Later", NotProven: "the outbound leg is not wired"}},
		Flows: []contract.ManifestFlow{
			{ID: "alpha", Title: "Alpha", Stage: "basic", Requires: []string{}, Source: &contract.ManifestSource{File: "flows/alpha.ts", Line: 3}, Steps: []contract.ManifestStep{
				{ID: "a1", Title: "A1", Kind: contract.StepKindRequest, Source: &contract.ManifestSource{File: "flows/alpha.ts", Line: 5}},
				{ID: "a2", Title: "A2", Kind: contract.StepKindRequest},
				{ID: "a3", Title: "A3", Kind: contract.StepKindWait},
			}},
			{ID: "beta", Title: "Beta", Stage: "basic", Once: true, Requires: []string{"alpha"}, Steps: []contract.ManifestStep{
				{ID: "b1", Title: "B1", Kind: contract.StepKindRequest},
				{ID: "b2", Title: "B2", Kind: contract.StepKindCapture},
			}},
			{ID: "gamma", Title: "Gamma", Stage: "basic", Requires: []string{}, Steps: []contract.ManifestStep{
				{ID: "g1", Title: "G1", Kind: contract.StepKindRequest},
			}},
		},
	}
}

func resultFixture() *contract.Result {
	r := &contract.Result{SchemaVersion: contract.RunnerSchemaVersion, Suite: "toy", Complete: true, StartedAt: "2026-10-19T01:00:00.000Z", FinishedAt: "2026-10-19T01:00:09.000Z"}
	for _, f := range manifestFixture().Flows {
		for _, s := range f.Steps {
			n := 0
			if s.Kind == contract.StepKindRequest {
				n = 1
			}
			r.Steps = append(r.Steps, contract.ResultStep{Flow: f.ID, ID: s.ID, Status: contract.StepStatusPassed, Requests: n})
		}
	}
	return r
}

func step(r *contract.Result, flow, id string) *contract.ResultStep {
	for i := range r.Steps {
		if r.Steps[i].Flow == flow && r.Steps[i].ID == id {
			return &r.Steps[i]
		}
	}
	panic("no step " + flow + "/" + id)
}

func failedAtA2(message string) *contract.Result {
	r := resultFixture()
	*step(r, "alpha", "a2") = contract.ResultStep{Flow: "alpha", ID: "a2", Status: contract.StepStatusFailed, Requests: 1, Message: message}
	for _, k := range [][2]string{{"alpha", "a3"}, {"beta", "b1"}, {"beta", "b2"}} {
		*step(r, k[0], k[1]) = contract.ResultStep{Flow: k[0], ID: k[1], Status: contract.StepStatusSkipped}
	}
	r.StoppedAt = &contract.StopPoint{Flow: "alpha", Step: "a2"}
	return r
}

func intp(n int) *int { return &n }

func base() Inputs {
	return Inputs{
		Manifest:   ManifestRead{Value: manifestFixture()},
		Result:     ResultRead{Value: resultFixture()},
		Boot:       &contract.BootVerdict{OK: true},
		Seal:       contract.SealPassed,
		RunnerCode: intp(0),
	}
}

func with(change func(in *Inputs)) Inputs {
	in := base()
	change(&in)
	return in
}

func masker(t testing.TB, known ...string) *mask.Masker {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: known})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func repoRoot(t testing.TB) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func fileText(t testing.TB, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

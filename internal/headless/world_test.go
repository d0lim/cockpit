package headless

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/config"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/procgroup"
	hubstages "github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/runnerbin"
	"github.com/d0lim/cockpit/internal/seal"
	"github.com/d0lim/cockpit/internal/version"
)

var fakeImage = runnerbin.Image{Tag: fakedocker.RunnerTag, ID: fakedocker.PinnedID, Arch: "arm64"}

func passingSteps() []contract.ResultStep {
	return []contract.ResultStep{
		{Flow: "seed", ID: "seed-users", Status: contract.StepStatusPassed, Requests: 1},
		{Flow: "health", ID: "health-ok", Status: contract.StepStatusPassed, Requests: 1},
		{Flow: "orders", ID: "create", Status: contract.StepStatusPassed, Requests: 2},
		{Flow: "facts", ID: "facts-read", Status: contract.StepStatusPassed, Requests: 1},
	}
}

func failingSteps(message string) ([]contract.ResultStep, *contract.StopPoint) {
	steps := passingSteps()
	steps[1].Status, steps[1].Message = contract.StepStatusFailed, message
	steps[2].Status, steps[2].Requests = contract.StepStatusSkipped, 0
	steps[3].Status, steps[3].Requests = contract.StepStatusSkipped, 0
	return steps, &contract.StopPoint{Flow: "health", Step: "health-ok"}
}

func newFake(mutate func(*fakedocker.Fake)) *fakedocker.Fake {
	f := &fakedocker.Fake{
		ConfigFixture: filepath.Join("..", "seal", "testdata", "config.toy.json"),
		Base:          config.Defaults().PortBase,
		Version:       version.String(),
		RunnerLogs:    "health › health-ok passed\ntoken " + planted + " leaked\n",
		EnvFiles: map[string]string{
			".env":                 "POSTGRES_USER=toy\nPOSTGRES_DB=toy\nPOSTGRES_PASSWORD=dbpass-0123456789abcdef\nAPP_TOKEN=toy_abcdefghijklmnopqrstuvwx\n",
			compose.PrivateEnvName: `{"local":{"appToken":"` + planted + `"}}`,
		},
		Steps: passingSteps(),
		Flows: []contract.ManifestFlow{
			{ID: "seed", Stage: "basic", Once: true, Requires: []string{}},
			{ID: "health", Stage: "basic", Requires: []string{}},
			{ID: "orders", Stage: "basic", Requires: []string{"health", "seed"}},
			{ID: "facts", Stage: "observe", Requires: []string{"orders"}},
		},
	}
	if mutate != nil {
		mutate(f)
	}
	return f
}

func canaryFor(f *fakedocker.Fake) func(int) (*seal.Canary, error) {
	return func(int) (*seal.Canary, error) {
		c, err := seal.ListenCanaryAt("127.0.0.1", 0)
		if err == nil {
			f.SetCanaryPort(c.Port())
		}
		return c, err
	}
}

func runFake(t *testing.T, project string, f *fakedocker.Fake, opts Options) (int, string) {
	t.Helper()
	return runFakeIn(context.Background(), t, project, f, opts)
}

func runFakeIn(ctx context.Context, t *testing.T, project string, f *fakedocker.Fake, opts Options) (int, string) {
	t.Helper()
	opts.Project = project
	var out bytes.Buffer
	code := Run(ctx, opts, Deps{
		Docker:       f.For(procgroup.Hooks{}),
		EnsureImage:  func(context.Context) (runnerbin.Image, error) { return fakeImage, nil },
		Home:         t.TempDir(),
		Out:          &out,
		Free:         func(int) bool { return true },
		TempDir:      t.TempDir(),
		Submitter:    "tester",
		ListenCanary: canaryFor(f),
		Limits:       testLimits(),
	})
	return code, out.String()
}

func testLimits() hubstages.Limits {
	limits := hubstages.DefaultLimits()
	limits.OTLPQuiet, limits.OTLPDrain = 50*time.Millisecond, time.Second
	return limits
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return doc
}

func archiveEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zipped, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string][]byte{}
	r := tar.NewReader(zipped)
	for {
		h, err := r.Next()
		if err == io.EOF {
			return entries
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		entries[h.Name] = data
	}
}

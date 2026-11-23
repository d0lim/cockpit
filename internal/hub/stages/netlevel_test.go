package stages

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/seal"
)

var netLevelMutations = []porttest.Mutation{
	{"the probes run unless the level is open", "return early from SealProbes for every level", "red: a sealed run starts no probe container", "TestTheSealStageRunsTheProbesAtSealedAndSkipsThemAtOpen"},
}

func TestNetLevelMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "netlevel_test.go", netLevelMutations)
}

type recordingDocker struct {
	mu   sync.Mutex
	runs []string
}

func (d *recordingDocker) Run(ctx context.Context, cmd compose.Command) (compose.Result, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.runs = append(d.runs, strings.Join(cmd.Args, " "))
	return compose.Result{}, nil
}

func sealStage(t *testing.T, level contract.NetworkLevel) ([]string, error, map[string]any) {
	t.Helper()
	l, err := compose.NewLayout(filepath.Join(t.TempDir(), "run"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := compose.OpenRun(l.Root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	decl := contract.Declaration{Network: level, Compose: contract.ComposeSection{Files: []contract.RelPath{"compose.yaml"}}}
	d := &recordingDocker{}
	p, err := OpenProject("r-1", l, decl, d, func(s string) string { return s }, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := &Env{RunID: "r-1", Loaded: compose.Loaded{Declaration: decl}, Layout: l, Root: root, Project: p, Base: 22000, Now: time.Now, Limits: DefaultLimits(),
		ListenCanary: func(int) (*seal.Canary, error) { return seal.ListenCanaryAt("127.0.0.1", 0) }}
	_, sealErr := e.SealProbes(context.Background())
	data, err := root.ReadFile(seal.FileName)
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]any
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	return d.runs, sealErr, file
}

func TestTheSealStageRunsTheProbesAtSealedAndSkipsThemAtOpen(t *testing.T) {
	runs, err, file := sealStage(t, contract.NetworkSealed)
	probeRuns := slices.DeleteFunc(slices.Clone(runs), func(r string) bool { return !strings.Contains(r, compose.RoleProbe) })
	if len(probeRuns) != 2 || err == nil || file["network"] != "sealed" {
		t.Fatalf("sealed: probe runs %q, err %v, network %v", probeRuns, err, file["network"])
	}
	runs, err, file = sealStage(t, contract.NetworkOpen)
	probes, _ := file["probes"].([]any)
	if len(runs) != 0 || err != nil || file["network"] != "open" || len(probes) != 0 {
		t.Fatalf("open: runs %q, err %v, network %v, probes %v", runs, err, file["network"], probes)
	}
}

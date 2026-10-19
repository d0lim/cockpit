package report

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/signal"
)

const runID = "run-20261019-010000-abc1234"

type fakeSignals struct {
	line    string
	err     error
	froze   int
	refused string
}

func (f *fakeSignals) Freeze(dir string) (signal.FreezeResult, error) {
	f.froze++
	if f.err != nil {
		return signal.FreezeResult{}, f.err
	}
	if err := os.MkdirAll(filepath.Join(dir, signal.Dir), 0o755); err != nil {
		return signal.FreezeResult{}, err
	}
	err := os.WriteFile(filepath.Join(dir, signal.Dir, "stream.jsonl"), []byte(f.line+"\n"), 0o644)
	return signal.FreezeResult{FrozenSeq: 1, Records: 1, Dropped: map[string]int64{"collector-cap": 4}}, err
}

func (f *fakeSignals) RefuseReads(reason string) error {
	f.refused = reason
	return nil
}

type fixture struct {
	runDir  string
	bundle  Bundle
	signals *fakeSignals
	patched *store.Meta
}

func newFixture(t *testing.T, m *mask.Masker) *fixture {
	t.Helper()
	runDir := filepath.Join(t.TempDir(), "runs", runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	in := with(func(in *Inputs) { in.Result.Value, in.RunnerCode = failedAtA2("seed "+planted), intp(1) })
	result, err := json.Marshal(in.Result.Value)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{runDir: runDir, signals: &fakeSignals{line: `{"seq":1,"body":"ready"}`}}
	slot := 2
	meta := store.Meta{ID: runID, SHA: strings.Repeat("a", 40), Suite: "toy", HostAPI: 1, CockpitVersion: "0.1.0", Submitter: "tester", CreatedAt: "2026-10-19T01:00:00.000Z", Slot: &slot,
		RunnerImage: &contract.RunnerImage{Tag: "cockpit-runner:0.1.0", ID: "sha256:" + strings.Repeat("b", 64)}}
	f.bundle = Bundle{
		RunDir: runDir,
		Meta:   meta,
		Inputs: in,
		Summary: SummaryInput{RunID: runID, Suite: "toy", Commit: meta.SHA, Harness: 1, HubCommit: "0.1.0", HostAPI: 1, CockpitVersion: "0.1.0", RunnerImage: *meta.RunnerImage,
			Ports: map[string]int{"app": 21003}, Versions: map[string]string{"docker": "28.0.0"}, Stages: map[string]contract.CollectStage{"toy-db": {Status: contract.CollectStageOK}},
			Runner: &contract.RunnerOutcome{ContainerID: "c1", Code: intp(1), Lines: []string{}}, GeneratedAt: "2026-10-19T01:00:10.000Z"},
		Files:   map[string][]byte{"e2e/result.json": result, "seal.json": []byte(`{"verdict":"passed"}`)},
		Mask:    m,
		Signals: f.signals,
		Patch: func(change func(*store.Meta)) error {
			next := meta
			change(&next)
			f.patched = &next
			return nil
		},
	}
	return f
}

func listFiles(t *testing.T, dir string) (files, dirs []string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() {
			dirs = append(dirs, rel)
		} else {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	return files, dirs
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

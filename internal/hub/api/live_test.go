package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/signal"
)

var liveMutations = []porttest.Mutation{
	{"live flow step messages pass the run masker", "skip the Mask loop in flow", "red: the planted value is in the step message", "TestLiveFlowAndSourceViewsPassTheRunMaskerAndBundleViewsComeFromTheBundle"},
	{"live source text passes the run masker", "return data without a.d.Mask(v.ID).Mask in source", "red: the planted value is in the source text", "TestLiveFlowAndSourceViewsPassTheRunMaskerAndBundleViewsComeFromTheBundle"},
	{"live source is served only for files the manifest points to", "drop the manifestFiles check in source", "red: a file the manifest does not name is served", "TestSourceRefusesFilesOutsideTheSuiteAndFilesTheManifestDoesNotName"},
	{"live source is refused without a run masker", "skip the nil Mask check in source", "red: the handler panics on the nil masker and the client gets no 409", "TestSourceRefusesFilesOutsideTheSuiteAndFilesTheManifestDoesNotName"},
	{"signal reads of a refused run answer READ_REFUSED", "map ErrReadRefused to a 500", "red: the refused run answers INTERNAL", "TestSignalReadsCarryTheFrozenSeqDroppedCountsCollectorStateAndAResyncMarker"},
}

func TestLiveMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "live_test.go", liveMutations)
}

type streamSource struct{ s *signal.Stream }

func (r streamSource) Read(_ string, req signal.ReadRequest) (signal.ReadResponse, error) {
	return r.s.Read(req)
}

type fixedCollectors []CollectorState

func (c fixedCollectors) Status(string) []CollectorState { return c }

func hubMasker(t *testing.T, known ...string) *mask.Masker {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: known})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSignalReadsCarryTheFrozenSeqDroppedCountsCollectorStateAndAResyncMarker(t *testing.T) {
	var src streamSource
	f := newFixture(t, func(d *Deps) {
		d.Signals = &src
		d.Collectors = fixedCollectors{{ID: "toy-db", State: collect.StateBackoff, LastOKAt: "2026-10-19T01:00:05.000Z", Failures: 3, Reason: "deadline"}}
	})
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictPassed}, doneUp))
	m := hubMasker(t)
	stream, err := signal.Open(f.runDir(rid(1)), signal.Options{Masker: func() *mask.Masker { return m }, Caps: signal.Caps{Tail: 2, PerCollector: 100, PerContainer: 100, CompactMin: 1 << 30}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stream.Close() })
	src.s = stream
	metric := contract.Signal{Kind: contract.SignalKindMetric, Collector: "toy-db", Metric: &contract.MetricBody{Name: "rows", Value: 1}}
	stream.Append(metric)
	if _, err := stream.Freeze(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		stream.Append(metric)
	}
	status, raw := f.get("/api/runs/" + rid(1) + "/signals?after=1")
	if status != http.StatusOK {
		t.Fatalf("%d %s", status, raw)
	}
	doc := validate(t, reflect.TypeFor[SignalsView](), raw)
	var view SignalsView
	json.Unmarshal(raw, &view)
	if !view.Frozen || view.FrozenSeq != 1 || view.Dropped[signal.ReasonTailCap] != 2 || view.Resync == nil || *view.Resync != (signal.Resync{From: 2, To: 3}) || len(view.Signals) != 2 {
		t.Fatalf("view %s", raw)
	}
	collectors := doc["collectors"].([]any)
	if len(collectors) != 1 || collectors[0].(map[string]any)["failures"] != float64(3) || collectors[0].(map[string]any)["state"] != "backoff" {
		t.Fatalf("collectors %v", collectors)
	}
	if status, raw := f.get("/api/runs/" + rid(1) + "/signals?after=3"); status != http.StatusOK || strings.Contains(string(raw), "resync") {
		t.Fatalf("canary without a gap: %d %s", status, raw)
	}
	stream.RefuseReads("redaction failed")
	status, raw = f.get("/api/runs/" + rid(1) + "/signals")
	var p Problem
	if json.Unmarshal(raw, &p); status != http.StatusConflict || p.Code != CodeReadRefused {
		t.Fatalf("refused: %d %s", status, raw)
	}
}

func writeRunnerFiles(t *testing.T, dir string, message string) {
	t.Helper()
	e2e := filepath.Join(dir, RunnerDir)
	os.MkdirAll(e2e, 0o700)
	manifest := contract.Manifest{SchemaVersion: contract.RunnerSchemaVersion, Suite: "toy", Stages: []contract.ManifestStage{{ID: "basic", Title: "Basic"}},
		Flows: []contract.ManifestFlow{{ID: "alpha", Title: "Alpha", Stage: "basic", Requires: []string{}, Source: &contract.ManifestSource{File: "flows/alpha.ts", Line: 1},
			Steps: []contract.ManifestStep{{ID: "a1", Title: "A1", Kind: contract.StepKindRequest}}}}}
	result := contract.Result{SchemaVersion: contract.RunnerSchemaVersion, Suite: "toy", Complete: true, StartedAt: "2026-10-19T01:00:00.000Z",
		Steps: []contract.ResultStep{{Flow: "alpha", ID: "a1", Status: contract.StepStatusFailed, Requests: 1, Message: message}}}
	for name, v := range map[string]any{"manifest.json": manifest, "result.json": result} {
		raw, _ := json.Marshal(v)
		os.WriteFile(filepath.Join(e2e, name), raw, 0o600)
	}
	scripts := filepath.Join(dir, ScriptsDir, "flows")
	os.MkdirAll(scripts, 0o700)
	os.WriteFile(filepath.Join(scripts, "alpha.ts"), []byte("const token = \""+planted+"\";\nexport default 1;\n"), 0o600)
	os.WriteFile(filepath.Join(scripts, "other.ts"), []byte("export default 2;\n"), 0o600)
}

func TestLiveFlowAndSourceViewsPassTheRunMaskerAndBundleViewsComeFromTheBundle(t *testing.T) {
	f := newFixture(t, func(d *Deps) { d.Mask = func(string) *mask.Masker { return hubMasker(t, planted) } })
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}, doneUp))
	dir := f.runDir(rid(1))
	writeRunnerFiles(t, dir, "status 500 with "+planted)
	status, raw := f.get("/api/runs/" + rid(1) + "/flows/alpha")
	if status != http.StatusOK || strings.Contains(string(raw), planted) || !strings.Contains(string(raw), "****") {
		t.Fatalf("live flow: %d %s", status, raw)
	}
	if validate(t, reflect.TypeFor[FlowView](), raw)["source"] != SourceLive {
		t.Fatal("source is not live")
	}
	status, raw = f.get("/api/runs/" + rid(1) + "/source?file=flows/alpha.ts")
	if status != http.StatusOK || strings.Contains(string(raw), planted) || !strings.Contains(string(raw), "****") {
		t.Fatalf("live source: %d %s", status, raw)
	}
	validate(t, reflect.TypeFor[SourceView](), raw)
	writeBundle(t, dir)
	status, raw = f.get("/api/runs/" + rid(1) + "/flows/alpha")
	if status != http.StatusOK || validate(t, reflect.TypeFor[FlowView](), raw)["source"] != SourceBundle {
		t.Fatalf("bundle flow: %d %s", status, raw)
	}
	if status, _ := f.get("/api/runs/" + rid(1) + "/flows/ghost"); status != http.StatusNotFound {
		t.Fatalf("missing flow: %d", status)
	}
}

func TestSourceRefusesFilesOutsideTheSuiteAndFilesTheManifestDoesNotName(t *testing.T) {
	var masker func(string) *mask.Masker
	f := newFixture(t, func(d *Deps) { d.Mask = func(id string) *mask.Masker { return masker(id) } })
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}, doneUp))
	writeRunnerFiles(t, f.runDir(rid(1)), "x")
	masker = func(string) *mask.Masker { return hubMasker(t, planted) }
	denied := map[string]int{"../../meta.json": 400, "/etc/passwd": 400, "flows/other.ts": 404}
	for file, want := range denied {
		if status, raw := f.get("/api/runs/" + rid(1) + "/source?file=" + file); status != want || strings.Contains(string(raw), "export default 2") {
			t.Errorf("%s: %d %s", file, status, raw)
		}
	}
	unmasked := newFixture(t, nil)
	unmasked.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}, doneUp))
	writeRunnerFiles(t, unmasked.runDir(rid(1)), "x")
	if status, raw := unmasked.get("/api/runs/" + rid(1) + "/source?file=flows/alpha.ts"); status != http.StatusConflict || strings.Contains(string(raw), planted) {
		t.Fatalf("without a masker: %d %s", status, raw)
	}
	if status, _ := f.get("/api/runs/" + rid(1) + "/source?file=flows/alpha.ts"); status != http.StatusOK {
		t.Fatalf("allowed file: %d", status)
	}
	if len(denied) != 3 {
		t.Fatal("case count")
	}
}

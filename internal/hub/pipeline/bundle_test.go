package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/signal"
)

var bundleMutations = []porttest.Mutation{
	{"the report stage writes a masked format-3 bundle", "skip report.Assemble in DefaultReport", "red: no report/summary.json", "TestAPassingRunWritesAFormat3BundleWithOwnerOnlyModesAndNoPlaintextSecret"},
	{"the boot verdict reaches the report", "pass no Boot to the report inputs", "red: the passing run closes as error with no boot verdict", "TestAPassingRunWritesAFormat3BundleWithOwnerOnlyModesAndNoPlaintextSecret"},
	{"a failed suite closes as failed with its stop point", "drop Result from the report inputs", "red: the verdict is error and no stoppedAt is written", "TestAFailedSuiteClosesAsFailedAndTheSummaryRecordsTheStopPoint"},
	{"a rescan hit discards the bundle, marks the meta and refuses signal reads", "rename report.partial without the rescan in report.Assemble", "red: report/ exists with the value planted in the frozen copy", "TestARescanHitDiscardsTheBundleMarksRedactionFailedAndRefusesSignalReads"},
}

func TestBundleMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "bundle_test.go", bundleMutations)
}

func summarySchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	r, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func readSummary(t *testing.T, runDir string) (map[string]any, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, report.DirReport, report.FileSummary))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc, string(data)
}

func assertBundle(t *testing.T, runDir string) string {
	t.Helper()
	doc, _ := readSummary(t, runDir)
	if err := summarySchema(t).Validate(doc); err != nil || doc["format"] != float64(3) {
		t.Fatalf("summary format %v: %v", doc["format"], err)
	}
	var all strings.Builder
	files := 0
	err := filepath.WalkDir(filepath.Join(runDir, report.DirReport), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		want := fs.FileMode(0o600)
		if d.IsDir() {
			want = 0o700
		} else {
			files++
			data, _ := os.ReadFile(path)
			all.Write(data)
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s mode %o", path, info.Mode().Perm())
		}
		return nil
	})
	if err != nil || files < 5 {
		t.Fatalf("walk: %v, %d files", err, files)
	}
	return all.String()
}

func TestAPassingRunWritesAFormat3BundleWithOwnerOnlyModesAndNoPlaintextSecret(t *testing.T) {
	w := newWorld(t, nil, nil)
	m := w.submit()
	if m.Verdict != lifecycle.VerdictPassed || m.Redaction != nil {
		t.Fatalf("state %+v notice %q redaction %+v", m.State, m.Notice, m.Redaction)
	}
	dir, _ := w.hub.Store().RunDir(m.ID)
	text := assertBundle(t, dir)
	if strings.Contains(text, planted) || !strings.Contains(text, "****") {
		t.Fatal("the bundle holds the planted value or masked nothing")
	}
	doc, _ := readSummary(t, dir)
	runner, _ := doc["runner"].(map[string]any)
	stages, _ := doc["stages"].(map[string]any)
	if doc["verdict"] != "passed" || doc["harness"] != float64(5) || doc["hubCommit"] == "" || runner == nil || stages["runner.manifest.json"] == nil {
		t.Fatalf("summary verdict %v harness %v hubCommit %v runner %v stages %v", doc["verdict"], doc["harness"], doc["hubCommit"], runner, stages)
	}
	for _, name := range []string{"e2e/manifest.json", "e2e/result.json", "seal.json", "meta.json", "not-proven.md", "signals/stream.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, report.DirReport, name)); err != nil {
			t.Errorf("bundle lacks %s: %v", name, err)
		}
	}
}

func TestAFailedSuiteClosesAsFailedAndTheSummaryRecordsTheStopPoint(t *testing.T) {
	steps := passingSteps()
	steps[1].Status, steps[1].Message = contract.StepStatusFailed, "expected status 200 but got 503"
	steps[2].Status, steps[2].Requests = contract.StepStatusSkipped, 0
	steps[3].Status, steps[3].Requests = contract.StepStatusSkipped, 0
	w := newWorld(t, func(f *fakedocker.Fake) {
		f.RunnerCode, f.Steps, f.StoppedAt = 1, steps, &contract.StopPoint{Flow: "health", Step: "health-ok"}
	}, nil)
	m := w.submit()
	if m.Verdict != lifecycle.VerdictFailed || !strings.Contains(m.Notice, "health › health-ok") {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	dir, _ := w.hub.Store().RunDir(m.ID)
	assertBundle(t, dir)
	doc, _ := readSummary(t, dir)
	stop, _ := doc["stoppedAt"].(map[string]any)
	if doc["verdict"] != "failed" || stop["flow"] != "health" || stop["step"] != "health-ok" {
		t.Fatalf("summary verdict %v stop %v", doc["verdict"], doc["stoppedAt"])
	}
}

type plantingSignals struct {
	*signal.Stream
}

func (p plantingSignals) Freeze(bundleDir string) (signal.FreezeResult, error) {
	res, err := p.Stream.Freeze(bundleDir)
	if err != nil {
		return res, err
	}
	f, err := os.OpenFile(filepath.Join(bundleDir, signal.Dir, "stream.jsonl"), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return res, err
	}
	defer f.Close()
	_, err = f.WriteString(`{"seq":99,"collector":"items","attrs":{"note":"` + planted + `"}}` + "\n")
	return res, err
}

func plantInFrozenCopy(ctx context.Context, in envops.ReportInput) (reconcile.ReportOutcome, string, error) {
	s, err := signal.Open(in.RunDir, signal.Options{Masker: func() *mask.Masker { return in.Masker }})
	if err != nil {
		return reconcile.ReportOutcome{}, "", err
	}
	defer s.Close()
	in.Signals = plantingSignals{s}
	return DefaultReport(ctx, in)
}

func TestARescanHitDiscardsTheBundleMarksRedactionFailedAndRefusesSignalReads(t *testing.T) {
	w := newWorld(t, nil, nil)
	w.hub.o.Reporter, w.hub.ops.Reporter = plantInFrozenCopy, plantInFrozenCopy
	m := w.submit()
	dir, _ := w.hub.Store().RunDir(m.ID)
	for _, name := range []string{report.DirReport, report.DirPartial} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s exists: %v", name, err)
		}
	}
	if m.Redaction == nil || m.Redaction.Status != report.RedactionFailed || len(m.Redaction.Files) != 1 || m.Redaction.Files[0] != "signals/stream.jsonl" {
		t.Fatalf("redaction %+v", m.Redaction)
	}
	ms, _ := mask.New(mask.Options{Mode: mask.Hub})
	s, err := signal.Open(dir, signal.Options{Masker: func() *mask.Masker { return ms }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Read(signal.ReadRequest{Limit: 10}); !errors.Is(err, signal.ErrReadRefused) {
		t.Fatalf("reads are not refused: %v", err)
	}
	clean := newWorld(t, nil, nil)
	cm := clean.submit()
	cdir, _ := clean.hub.Store().RunDir(cm.ID)
	if _, err := os.Stat(filepath.Join(cdir, report.DirReport)); err != nil || cm.Redaction != nil {
		t.Fatalf("canary: a clean run has no bundle: %v %+v", err, cm.Redaction)
	}
}

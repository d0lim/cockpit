package report

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/signal"
)

var bundleMutations = []porttest.Mutation{
	{"every bundle file is masked as it is written", "write files[name] without m.Mask in writeMasked", "red: the planted seed is in e2e/result.json and the bundle is discarded", "TestABundleWithPlantedValuesIsMaskedAndRenamedToReportWith700And600"},
	{"bundle permissions are forced to 700 and 600", "skip chmodTree in writeMasked", "red: signals/stream.jsonl keeps mode 644", "TestABundleWithPlantedValuesIsMaskedAndRenamedToReportWith700And600"},
	{"the rescan runs before the rename", "rename without calling mask.Rescan", "red: report/ exists and holds the planted value", "TestAPlantedValueLeftAtRescanDiscardsTheBundleRecordsThePathsAndRefusesReads"},
	{"a rescan hit removes the partial bundle", "skip os.RemoveAll in refuse", "red: report.partial/ is left with the planted value", "TestAPlantedValueLeftAtRescanDiscardsTheBundleRecordsThePathsAndRefusesReads"},
	{"a rescan hit records redaction failed in the run meta", "skip Patch in refuse", "red: the meta has no redaction", "TestAPlantedValueLeftAtRescanDiscardsTheBundleRecordsThePathsAndRefusesReads"},
	{"a rescan hit refuses the run's signal reads", "skip RefuseReads in refuse", "red: the real stream still answers reads", "TestARealStreamFrozenWithAnOlderValueListIsDiscardedAndItsReadsAreRefused"},
	{"the returned judgement is masked", "return the judgement without maskJudgement", "red: the planted seed is in the outcome's second reason", "TestABundleWithPlantedValuesIsMaskedAndRenamedToReportWith700And600"},
	{"bundle paths stay inside the bundle and off reserved names", "drop the IsLocal and reserved checks in Assemble", "red: ../escape.json is written beside the run", "TestBundlePathsOutsideTheBundleOrOnReservedNamesAreRefusedBeforeAnythingIsWritten"},
}

func TestBundleMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "bundle_test.go", bundleMutations)
}

func TestABundleWithPlantedValuesIsMaskedAndRenamedToReportWith700And600(t *testing.T) {
	f := newFixture(t, masker(t, planted))
	out, err := Assemble(f.bundle)
	if err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(f.runDir, DirReport)
	if out.Dir != report || out.Redaction != nil || exists(filepath.Join(f.runDir, DirPartial)) || out.Judgement.Verdict != contract.VerdictFailed {
		t.Fatalf("outcome %+v", out)
	}
	files, dirs := listFiles(t, report)
	want := []string{"e2e/result.json", "meta.json", "not-proven.md", "seal.json", "signals/stream.jsonl", "summary.json"}
	if !slices.Equal(files, want) {
		t.Fatalf("files %q", files)
	}
	for _, d := range dirs {
		if info, _ := os.Stat(filepath.Join(report, d)); info.Mode().Perm() != 0o700 {
			t.Errorf("dir %s mode %v", d, info.Mode().Perm())
		}
	}
	for _, name := range files {
		info, _ := os.Stat(filepath.Join(report, name))
		text := fileText(t, filepath.Join(report, name))
		if info.Mode().Perm() != 0o600 || strings.Contains(text, planted) {
			t.Errorf("%s: mode %v, planted present %v", name, info.Mode().Perm(), strings.Contains(text, planted))
		}
	}
	for _, name := range []string{"e2e/result.json", "summary.json"} {
		if !strings.Contains(fileText(t, filepath.Join(report, name)), "seed ****"+planted[len(planted)-4:]) {
			t.Errorf("%s: masked marker absent", name)
		}
	}
	if rep, err := mask.Rescan(report, f.bundle.Mask.Mask); err != nil || len(rep.Left) != 0 || rep.Scanned != 6 {
		t.Fatalf("rescan %+v %v", rep, err)
	}
	if strings.Contains(strings.Join(out.Judgement.Reasons, " "), planted) || !strings.Contains(out.Judgement.Reasons[1], "****") {
		t.Fatalf("outcome reasons are not masked: %q", out.Judgement.Reasons)
	}
	if f.patched != nil || f.signals.refused != "" || f.signals.froze != 1 || len(dirs) != 3 {
		t.Fatalf("patched %v, refused %q, froze %d, dirs %q", f.patched, f.signals.refused, f.signals.froze, dirs)
	}
}

func TestAPlantedValueLeftAtRescanDiscardsTheBundleRecordsThePathsAndRefusesReads(t *testing.T) {
	f := newFixture(t, masker(t, planted))
	f.signals.line = `{"seq":1,"body":"issued ` + planted + `"}`
	out, err := Assemble(f.bundle)
	if err != nil {
		t.Fatal(err)
	}
	if out.Dir != "" || exists(filepath.Join(f.runDir, DirReport)) || exists(filepath.Join(f.runDir, DirPartial)) {
		t.Fatalf("bundle kept: %+v", out)
	}
	want := []string{"signals/stream.jsonl"}
	if out.Redaction == nil || out.Redaction.Status != RedactionFailed || !slices.Equal(out.Redaction.Files, want) {
		t.Fatalf("redaction %+v", out.Redaction)
	}
	if f.patched == nil || f.patched.Redaction == nil || !slices.Equal(f.patched.Redaction.Files, want) {
		t.Fatalf("meta not patched: %+v", f.patched)
	}
	if f.signals.refused != RefusedRedaction || out.Judgement.Verdict != contract.VerdictFailed {
		t.Fatalf("refused %q, verdict %s", f.signals.refused, out.Judgement.Verdict)
	}
	for _, p := range f.patched.Redaction.Files {
		if filepath.IsAbs(p) || strings.Contains(p, f.runDir) {
			t.Fatalf("path is not inside the bundle: %s", p)
		}
	}
}

func TestARealStreamFrozenWithAnOlderValueListIsDiscardedAndItsReadsAreRefused(t *testing.T) {
	f := newFixture(t, masker(t, planted))
	stale := masker(t)
	stream, err := signal.Open(f.runDir, signal.Options{Masker: func() *mask.Masker { return stale }})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if _, err := stream.Append(contract.Signal{Kind: contract.SignalKindEvent, Collector: "toy-db", Event: &contract.EventBody{Body: "issued " + planted}}); err != nil {
		t.Fatal(err)
	}
	if resp, err := stream.Read(signal.ReadRequest{Limit: 10}); err != nil || len(resp.Signals) != 1 {
		t.Fatalf("canary: the stream does not answer before the report: %v", err)
	}
	f.bundle.Signals = stream
	out, err := Assemble(f.bundle)
	if err != nil {
		t.Fatal(err)
	}
	if out.Dir != "" || out.Redaction == nil || !slices.Equal(out.Redaction.Files, []string{"signals/stream.jsonl"}) {
		t.Fatalf("outcome %+v", out)
	}
	if _, err := stream.Read(signal.ReadRequest{Limit: 10}); !errors.Is(err, signal.ErrReadRefused) {
		t.Fatalf("reads are not refused: %v", err)
	}
	if out.Frozen.Records != 1 {
		t.Fatalf("frozen %+v", out.Frozen)
	}
}

func TestBundlePathsOutsideTheBundleOrOnReservedNamesAreRefusedBeforeAnythingIsWritten(t *testing.T) {
	denied := []string{"../escape.json", "/abs/escape.json", "summary.json", "meta.json", "not-proven.md", "signals/extra.jsonl", "e2e/../../escape.json"}
	for _, name := range denied {
		f := newFixture(t, masker(t, planted))
		f.bundle.Files[name] = []byte("x")
		if _, err := Assemble(f.bundle); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if exists(filepath.Join(f.runDir, DirPartial)) || exists(filepath.Join(filepath.Dir(f.runDir), "escape.json")) || f.signals.froze != 0 {
			t.Errorf("%s: something was written", name)
		}
	}
	f := newFixture(t, masker(t, planted))
	f.bundle.Files["collectors/toy-db/rows.json"] = []byte(`["row"]`)
	if out, err := Assemble(f.bundle); err != nil || !exists(filepath.Join(out.Dir, "collectors/toy-db/rows.json")) {
		t.Fatalf("allowed nested path: %v", err)
	}
	if len(denied) != 7 {
		t.Fatal("case count")
	}
}

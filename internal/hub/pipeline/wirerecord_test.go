package pipeline

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/wire"
)

var wireRecordMutations = []porttest.Mutation{
	{"the record masker holds the run's values", "drop the run's values from recordMasker", "red: the planted run value stays in calls.jsonl", "TestARecordingRunStoresItsMaskedRecordInTheBundle"},
	{"the record masker holds the values the runner captured", "drop capturedValues from recordMasker", "red: the six-character value stays in calls.jsonl", "TestARecordingRunStoresItsMaskedRecordInTheBundle"},
	{"the record masker reaches six-character values", "build the record masker in mask.Hub mode", "red: calls.jsonl reads ****3a9c where it must read ****", "TestARecordingRunStoresItsMaskedRecordInTheBundle"},
	{"the record joins the bundle after the rescan", "move <run>/wire into report.partial before mask.Rescan in report.Assemble", "red: report/ is absent and redaction lists wire/suite/calls.jsonl", "TestAPEMPairAcrossCallsAndAPublicFieldLeaveTheBundleAndTheRecordStanding"},
	{"public fields reach the record masker", "drop WithPublicFields from recordMasker", "red: the public token reads ****", "TestAPEMPairAcrossCallsAndAPublicFieldLeaveTheBundleAndTheRecordStanding"},
	{"each stored call is proven", "store the cleaned call instead of the result of Prove in taking.addLine", "red: the call with the PEM pair is stored whole", "TestACallFailingItsProofIsStoredAsAStubAndTheReportStands"},
	{"a missing values file withholds the record", "return no values when wire.private.env.json is missing in capturedValues", "red: the record is stored with the captured value in it", "TestAMissingValuesFileNextToTheManifestWithholdsTheRecord"},
	{"the recorder's refusal reaches the hub log masked", "pass err.Error() without ops.Mask in RunWire.warn", "red: the planted value is in the log", "TestARefusedTakeIsLostAndItsReasonReachesTheHubLogMasked"},
	{"a run without wire reaches no recorder", "set ReportInput.Wire for every run in AddCollected", "red: a plain run makes a wire-ctl exec and leaves a wire directory", "TestARunWithoutWireReachesNoRecorderAndLeavesNoRecord"},
}

func TestWireRecordMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "wirerecord_test.go", wireRecordMutations)
}

func assertModes(t *testing.T, root string) int {
	t.Helper()
	files := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s mode %o, want %o", path, info.Mode().Perm(), want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestARecordingRunStoresItsMaskedRecordInTheBundle(t *testing.T) {
	w := wireWorld(t, nil)
	m, dir := w.reportedWire()
	if m.Verdict != lifecycle.VerdictPassed || m.Redaction != nil {
		t.Fatalf("state %+v notice %q redaction %+v", m.State, m.Notice, m.Redaction)
	}
	assertBundle(t, dir)
	if files := assertModes(t, bundleRecord(dir)); files != 2 {
		t.Fatalf("the record holds %d files", files)
	}
	absent(t, stagedRecord(dir))
	files := wireFiles(t, bundleRecord(dir))
	calls, index := files[filepath.Join("suite", wire.CallsFile)], files[filepath.Join("suite", wire.IndexFile)]
	if calls == "" || index == "" {
		t.Fatalf("record files %v", slices.Collect(maps.Keys(files)))
	}
	for _, secret := range []string{planted, capturedSix} {
		for name, text := range files {
			if strings.Contains(text, secret) {
				t.Errorf("%s holds %q", name, secret)
			}
		}
		if strings.Contains(calls, secret) || !strings.Contains(calls, "token ****") || !strings.Contains(calls, "captured ****") {
			t.Fatalf("calls.jsonl must read **** in place of %q:\n%s", secret, calls)
		}
	}
	status := summaryWire(t, dir)
	if status.State != wire.StateTaken || status.Take != wire.SuiteTake || status.Calls != 2 || status.WithheldCalls != 0 {
		t.Fatalf("summary.wire %+v", status)
	}
	var meta contract.BundleMeta
	data, err := os.ReadFile(filepath.Join(dir, report.DirReport, report.FileMeta))
	if err != nil || json.Unmarshal(data, &meta) != nil || !meta.Wire {
		t.Fatalf("meta.json wire %v: %v\n%s", meta.Wire, err, data)
	}
	read, err := wire.OpenStore(dir).Index(wire.SuiteTake)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Heads) != 2 || read.Heads[0].Link.Kind != wire.LinkExact || read.Steps[read.Heads[0].Link.Step] != (wire.StepRef{Flow: "orders", Step: "create", Status: contract.StepStatusPassed}) {
		t.Fatalf("heads %+v steps %+v", read.Heads, read.Steps)
	}
	if len(read.PublicFields) != 1 || read.PublicFields[0].Key != "token" {
		t.Fatalf("public fields %+v", read.PublicFields)
	}
	if got := unreadLines(t, dir); slices.ContainsFunc(got, func(l string) bool { return strings.HasPrefix(l, stageWire) }) {
		t.Fatalf("a taken record added an unread line: %q", got)
	}
}

func TestAPEMPairAcrossCallsAndAPublicFieldLeaveTheBundleAndTheRecordStanding(t *testing.T) {
	publicBody := wire.Body{Kind: wire.BodyJSON, JSON: map[string]any{"token": publicToken}}
	w := wireWorld(t, func(f *fakedocker.Fake) {
		f.Wire.Calls = []wire.Call{
			wireCall(1, textBody(pemBegin), textBody("first")),
			wireCall(2, textBody("second"), textBody(pemEnd)),
			wireCall(3, publicBody, textBody("third")),
		}
	})
	m, dir := w.reportedWire()
	if m.Redaction != nil || m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("state %+v notice %q redaction %+v", m.State, m.Notice, m.Redaction)
	}
	present(t, filepath.Join(dir, report.DirReport, report.FileSummary))
	calls := wireFiles(t, bundleRecord(dir))[filepath.Join("suite", wire.CallsFile)]
	for _, want := range []string{pemBegin, pemEnd, publicToken} {
		if !strings.Contains(calls, want) {
			t.Errorf("the record lost %q:\n%s", want, calls)
		}
	}
	if status := summaryWire(t, dir); status.State != wire.StateTaken || status.Calls != 3 || status.WithheldCalls != 0 {
		t.Fatalf("summary.wire %+v", status)
	}
}

func TestACallFailingItsProofIsStoredAsAStubAndTheReportStands(t *testing.T) {
	w := wireWorld(t, func(f *fakedocker.Fake) {
		f.Wire.Calls = []wire.Call{
			wireCall(1, textBody(pemBegin+"\nkeep-out-request"), textBody("keep-out-response\n"+pemEnd)),
			wireCall(2, textBody("visible request"), textBody("visible response")),
		}
	})
	m, dir := w.reportedWire()
	if m.Verdict != lifecycle.VerdictPassed || m.Redaction != nil {
		t.Fatalf("state %+v notice %q redaction %+v", m.State, m.Notice, m.Redaction)
	}
	if status := summaryWire(t, dir); status.State != wire.StateTaken || status.Calls != 2 || status.WithheldCalls != 1 {
		t.Fatalf("summary.wire %+v", status)
	}
	store := wire.OpenStore(dir)
	stub, head, err := store.Call(wire.SuiteTake, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !stub.Withheld || !head.Withheld || stub.Path != "" || stub.Request.Body.Kind != "" || stub.Method != "POST" {
		t.Fatalf("stub %+v head %+v", stub, head)
	}
	kept, _, err := store.Call(wire.SuiteTake, 2)
	if err != nil || kept.Withheld || kept.Request.Body.Text != "visible request" {
		t.Fatalf("canary: the second call %+v: %v", kept, err)
	}
	calls := wireFiles(t, bundleRecord(dir))[filepath.Join("suite", wire.CallsFile)]
	for _, gone := range []string{"keep-out", "BEGIN", "END"} {
		if strings.Contains(calls, gone) {
			t.Errorf("%q reached the record:\n%s", gone, calls)
		}
	}
}

func TestAMissingValuesFileNextToTheManifestWithholdsTheRecord(t *testing.T) {
	w := wireWorld(t, func(f *fakedocker.Fake) { f.Wire.NoValues = true })
	m, dir := w.reportedWire()
	if m.Verdict != lifecycle.VerdictPassed || m.Redaction != nil {
		t.Fatalf("state %+v notice %q redaction %+v", m.State, m.Notice, m.Redaction)
	}
	if status := summaryWire(t, dir); status.State != wire.StateWithheld || status.Calls != 0 {
		t.Fatalf("summary.wire %+v", status)
	}
	oneUnreadWireLine(t, dir, wire.StateWithheld)
	absent(t, bundleRecord(dir))
	absent(t, stagedRecord(dir))
	if taken := w.fake.Wire.Taken(); len(taken) != 0 {
		t.Fatalf("a withheld record was still read: %v", taken)
	}
	present(t, filepath.Join(dir, "work", "e2e", "manifest.json"))
}

func TestARunWithoutWireReachesNoRecorderAndLeavesNoRecord(t *testing.T) {
	w := wireWorld(t, nil)
	m := w.submit()
	if m.Wire || m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("wire %v state %+v notice %q", m.Wire, m.State, m.Notice)
	}
	dir, _ := w.hub.Store().RunDir(m.ID)
	if r := w.sched.Exec(m.ID, "health", tester); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	calls := w.fake.Calls()
	if len(calls) == 0 || slices.ContainsFunc(calls, func(c fakedocker.Call) bool { return slices.Contains(c.Args, "wire-ctl") }) {
		t.Fatalf("%d docker calls, wire-ctl among them: %v", len(calls), w.verbs())
	}
	absent(t, stagedRecord(dir))
	absent(t, bundleRecord(dir))
	doc, raw := readSummary(t, dir)
	if _, has := doc["wire"]; has || doc["verdict"] != "passed" {
		t.Fatalf("the summary of a plain run: %s", raw)
	}
	data, err := os.ReadFile(filepath.Join(dir, report.DirReport, report.FileMeta))
	if err != nil || strings.Contains(string(data), `"wire"`) || !strings.Contains(string(data), `"id"`) {
		t.Fatalf("meta.json %v:\n%s", err, data)
	}
}

func TestARefusedTakeIsLostAndItsReasonReachesTheHubLogMasked(t *testing.T) {
	var logged lockedBuffer
	w := newWorldWith(t, func(o *Options) { o.Log = slog.New(slog.NewJSONHandler(&logged, nil)) }, func(f *fakedocker.Fake) {
		f.Steps, f.Wire.Calls, f.Wire.Values = tracedSteps(), secretCalls(), []string{capturedSix}
		f.Wire.Refusal = "take refused near " + planted
	}, declareWire(t))
	m, dir := w.reportedWire()
	if m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	if status := summaryWire(t, dir); status.State != wire.StateLost {
		t.Fatalf("summary.wire %+v", status)
	}
	oneUnreadWireLine(t, dir, wire.StateLost)
	text := logged.String()
	if strings.Contains(text, planted) || !strings.Contains(text, "wire-ctl take was refused") || !strings.Contains(text, "take refused near ****") {
		t.Fatalf("the hub log must hold the masked reason:\n%s", text)
	}
}

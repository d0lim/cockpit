package pipeline

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/exec"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/wire"
)

var wireExecMutations = []porttest.Mutation{
	{"an exec is armed before its runner starts", "move armWire after RunRunner in Execer.Run", "red: the arm follows compose run", "TestAnExecArmsItsTakeFirstAndStoresItNextToTheSuiteRecord"},
	{"an exec take links against that exec's result", "pass no result in execRecording.finish", "red: the exec take has no steps and no exact link", "TestAnExecArmsItsTakeFirstAndStoresItNextToTheSuiteRecord"},
	{"a recorder failure does not fail the exec", "return a Fail from Execer.Run when the take is not taken", "red: the exec answers with a failure", "TestARecorderFailureBecomesAnExecOutputLineAndTheExecStillAnswers"},
	{"a runner that fails to run still gets its take", "return before finish when RunRunner fails", "red: the armed take is never read", "TestAnExecWhoseRunnerFailsStoresItsTakeAndKeepsItsOwnError"},
	{"the failure text passes the run masker", "drop r.Mask from the reason and p.Mask from the failure tail in RunRunner", "red: the planted value is in the reason", "TestAnExecWhoseRunnerFailsStoresItsTakeAndKeepsItsOwnError"},
	{"a suite record not stored yet is stored before the arm", "arm without trying the suite take", "red: the arm comes first and the suite take is never stored", "TestAnExecStoresAPendingSuiteRecordBeforeItArms"},
	{"an unstored suite record stops the arm", "arm whatever the suite take answered", "red: the arm erases the record the recorder still holds", "TestAnExecThatCannotStoreThePendingSuiteRecordRunsUnrecordedAndALaterExecStoresIt"},
	{"a restarted recorder holds no suite record worth keeping", "treat every state but taken as worth keeping", "red: no exec of the environment is recorded", "TestARecorderThatRestartedStillRecordsTheExecsAfterIt"},
}

func TestWireExecMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "wireexec_test.go", wireExecMutations)
}

func TestAnExecArmsItsTakeFirstAndStoresItNextToTheSuiteRecord(t *testing.T) {
	w := wireWorld(t, nil)
	m, dir := w.reportedWire()
	calls := len(w.fake.Calls())
	r := w.sched.Exec(m.ID, "health", tester)
	if r.Fail != nil {
		t.Fatal(r.Fail)
	}
	armed := w.fake.Wire.Armed()
	if len(armed) != 1 || !wire.ValidTake(armed[0]) || !strings.HasSuffix(armed[0], "-health") {
		t.Fatalf("armed %v", armed)
	}
	since := w.fake.Calls()[calls:]
	armAt := slices.IndexFunc(since, func(c fakedocker.Call) bool { return slices.Contains(c.Args, "arm") })
	runAt := slices.IndexFunc(since, func(c fakedocker.Call) bool { return fakedocker.Verb(c.Args) == "compose run "+compose.RoleRunner })
	takeAt := slices.IndexFunc(since, func(c fakedocker.Call) bool {
		return slices.Contains(c.Args, "wire-ctl") && slices.Contains(c.Args, "take")
	})
	if armAt < 0 || runAt < armAt || takeAt < runAt {
		t.Fatalf("arm at %d, runner at %d, take at %d", armAt, runAt, takeAt)
	}
	store := wire.OpenStore(dir)
	takes, err := store.Takes()
	if err != nil || !slices.Equal(takes, []string{armed[0], wire.SuiteTake}) {
		t.Fatalf("takes %v: %v", takes, err)
	}
	index, err := store.Index(armed[0])
	if err != nil || index.Status.State != wire.StateTaken || len(index.Heads) != 2 {
		t.Fatalf("exec index %+v: %v", index, err)
	}
	if index.Heads[0].Link.Kind != wire.LinkExact || index.Steps[index.Heads[0].Link.Step] != (wire.StepRef{Flow: "orders", Step: "create", Status: contract.StepStatusPassed}) {
		t.Fatalf("exec link %+v steps %+v", index.Heads[0].Link, index.Steps)
	}
	text := wireFiles(t, filepath.Join(bundleRecord(dir), armed[0]))[wire.CallsFile]
	if strings.Contains(text, planted) || strings.Contains(text, capturedSix) || !strings.Contains(text, "token ****") {
		t.Fatalf("the exec take must read **** in place of the values:\n%s", text)
	}
	view := r.Result.(exec.View)
	if slices.ContainsFunc(view.Lines, func(l string) bool { return strings.HasPrefix(l, "wire record") }) {
		t.Fatalf("a taken exec added a line: %q", view.Lines)
	}
}

func TestARecorderFailureBecomesAnExecOutputLineAndTheExecStillAnswers(t *testing.T) {
	cases := map[string]struct {
		edit func(*fakedocker.Fake)
		says string
	}{
		"gone before the arm":    {func(f *fakedocker.Fake) { f.Wire.Gone = true }, "could not be armed"},
		"a stream broken twice":  {func(f *fakedocker.Fake) { f.Wire.Broken = 2 }, string(wire.StateFailed) + ":"},
		"no values next to them": {func(f *fakedocker.Fake) { f.Wire.NoValues = true }, string(wire.StateWithheld) + ":"},
	}
	executed := 0
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			w := wireWorld(t, nil)
			m, _ := w.reportedWire()
			c.edit(w.fake)
			r := w.sched.Exec(m.ID, "health", tester)
			if r.Fail != nil {
				t.Fatalf("the exec failed: %+v", r.Fail)
			}
			view := r.Result.(exec.View)
			if view.ExitCode == nil || *view.ExitCode != 0 || len(view.Flows) == 0 {
				t.Fatalf("view %+v", view)
			}
			if !slices.ContainsFunc(view.Lines, func(l string) bool { return strings.HasPrefix(l, "wire record") && strings.Contains(l, c.says) }) {
				t.Fatalf("lines %q, want one saying %q", view.Lines, c.says)
			}
		})
		executed++
	}
	if executed != 3 {
		t.Fatalf("executed %d cases", executed)
	}
}

func wireLines(view exec.View) []string {
	var lines []string
	for _, l := range view.Lines {
		if strings.HasPrefix(l, "wire record") {
			lines = append(lines, l)
		}
	}
	return lines
}

func verbIndex(calls []fakedocker.Call, verb, take string) int {
	return slices.IndexFunc(calls, func(c fakedocker.Call) bool {
		return slices.Contains(c.Args, "wire-ctl") && slices.Contains(c.Args, verb) && (take == "" || slices.Contains(c.Args, take))
	})
}

func TestAnExecWhoseRunnerFailsStoresItsTakeAndKeepsItsOwnError(t *testing.T) {
	w := wireWorld(t, nil)
	m, dir := w.reportedWire()
	w.fake.FailVerb = map[string]bool{"compose run " + compose.RoleRunner: true}
	w.fake.FailStderr = " near " + planted
	r := w.sched.Exec(m.ID, "health", tester)
	if r.Fail == nil || !strings.Contains(r.Fail.Reason, "exec failed") || !strings.Contains(r.Fail.Reason, "failed on purpose") {
		t.Fatalf("the exec's own error is gone: %+v", r.Fail)
	}
	if strings.Contains(r.Fail.Reason, planted) || !strings.Contains(r.Fail.Reason, "****") {
		t.Fatalf("reason %q", r.Fail.Reason)
	}
	armed := w.fake.Wire.Armed()
	if len(armed) != 1 || !slices.Contains(w.fake.Wire.Taken(), armed[0]) {
		t.Fatalf("armed %v, taken %v", armed, w.fake.Wire.Taken())
	}
	index, err := wire.OpenStore(dir).Index(armed[0])
	if err != nil || index.Status.State != wire.StateTaken || len(index.Heads) != 2 {
		t.Fatalf("exec index %+v: %v", index, err)
	}
	w.fake.Wire.Broken = 2
	r = w.sched.Exec(m.ID, "health", tester)
	if r.Fail == nil || !strings.Contains(r.Fail.Reason, "failed on purpose") || !strings.Contains(r.Fail.Reason, "wire record: take") {
		t.Fatalf("a take that also failed: %+v", r.Fail)
	}
}

func TestAnExecStoresAPendingSuiteRecordBeforeItArms(t *testing.T) {
	w := wireWorld(t, func(f *fakedocker.Fake) { f.Wire.Broken = 2 })
	m, dir := w.reportedWire()
	store := wire.OpenStore(dir)
	if status := summaryWire(t, dir); status.State != wire.StateFailed {
		t.Fatalf("canary: summary.wire %+v", status)
	}
	if takes, err := store.Takes(); err != nil || len(takes) != 0 {
		t.Fatalf("canary: takes %v: %v", takes, err)
	}
	calls := len(w.fake.Calls())
	r := w.sched.Exec(m.ID, "health", tester)
	if r.Fail != nil {
		t.Fatal(r.Fail)
	}
	armed := w.fake.Wire.Armed()
	if len(armed) != 1 {
		t.Fatalf("armed %v", armed)
	}
	since := w.fake.Calls()[calls:]
	suiteAt, armAt := verbIndex(since, "take", wire.SuiteTake), verbIndex(since, "arm", "")
	if suiteAt < 0 || armAt < suiteAt {
		t.Fatalf("suite take at %d, arm at %d", suiteAt, armAt)
	}
	takes, err := store.Takes()
	if err != nil || !slices.Equal(takes, []string{armed[0], wire.SuiteTake}) {
		t.Fatalf("takes %v: %v", takes, err)
	}
	index, err := store.Index(wire.SuiteTake)
	if err != nil || index.Heads[0].Link.Kind != wire.LinkExact {
		t.Fatalf("the suite take was not linked to the run's steps: %+v %v", index.Heads, err)
	}
	if lines := wireLines(r.Result.(exec.View)); len(lines) != 0 {
		t.Fatalf("lines %q", lines)
	}
}

func TestAnExecThatCannotStoreThePendingSuiteRecordRunsUnrecordedAndALaterExecStoresIt(t *testing.T) {
	w := wireWorld(t, func(f *fakedocker.Fake) { f.Wire.Broken = 4 })
	m, dir := w.reportedWire()
	r := w.sched.Exec(m.ID, "health", tester)
	if r.Fail != nil {
		t.Fatal(r.Fail)
	}
	view := r.Result.(exec.View)
	if view.ExitCode == nil || *view.ExitCode != 0 || len(view.Flows) == 0 {
		t.Fatalf("the exec did not run: %+v", view)
	}
	lines := wireLines(view)
	if len(lines) != 1 || !strings.Contains(lines[0], "suite") || !strings.Contains(lines[0], "not recorded") {
		t.Fatalf("lines %q", lines)
	}
	if armed := w.fake.Wire.Armed(); len(armed) != 0 {
		t.Fatalf("the recorder was armed and lost the suite record: %v", armed)
	}
	if r := w.sched.Exec(m.ID, "health", tester); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	armed := w.fake.Wire.Armed()
	takes, err := wire.OpenStore(dir).Takes()
	if err != nil || len(armed) != 1 || !slices.Equal(takes, []string{armed[0], wire.SuiteTake}) {
		t.Fatalf("armed %v, takes %v: %v", armed, takes, err)
	}
}

func TestARecorderThatRestartedStillRecordsTheExecsAfterIt(t *testing.T) {
	w := wireWorld(t, func(f *fakedocker.Fake) { f.Wire.Since = time.Date(2026, 10, 19, 1, 0, 5, 0, time.UTC) })
	m, dir := w.reportedWire()
	if status := summaryWire(t, dir); status.State != wire.StateLost {
		t.Fatalf("canary: summary.wire %+v", status)
	}
	r := w.sched.Exec(m.ID, "health", tester)
	if r.Fail != nil {
		t.Fatal(r.Fail)
	}
	armed := w.fake.Wire.Armed()
	takes, err := wire.OpenStore(dir).Takes()
	if err != nil || len(armed) != 1 || !slices.Equal(takes, []string{armed[0]}) {
		t.Fatalf("armed %v, takes %v: %v", armed, takes, err)
	}
	if lines := wireLines(r.Result.(exec.View)); len(lines) != 0 {
		t.Fatalf("lines %q", lines)
	}
}

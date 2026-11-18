package pipeline

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

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

package runner

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

func execArgs(targets ...string) []string {
	var args []string
	for _, t := range targets {
		args = append(args, "--flow", t)
	}
	return args
}

func (w *world) execOut(name string) string {
	return filepath.Join(w.dir, "exec", name)
}

func (w *world) readExec(name string) contract.ExecResult {
	var r contract.ExecResult
	w.read(filepath.Join("exec", name), &r)
	return r
}

func TestAE3OnceFlowWithLedgerRecordDoesNotRunAndExitsTwoWithReason(t *testing.T) {
	w := newWorld(t, core, nil)
	w.seed(map[string]contract.FlowStatus{"alpha": "passed"}, map[string]string{"alpha.id": "id-a"})
	out := w.execOut("t-alpha.json")
	got := w.run(map[string]string{"E2E_EXEC_OUT": out}, execArgs("alpha")...)
	if got.code != 2 || len(w.paths()) != 0 || len(got.stdout) != 0 {
		t.Fatalf("exit %d calls %v stdout %v", got.code, w.paths(), got.stdout)
	}
	joined := strings.Join(got.stderr, "\n")
	if !strings.Contains(joined, "alpha") || !strings.Contains(joined, "already ran") {
		t.Fatalf("stderr %q", joined)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("the exec file was created")
	}
}

func TestNeverStartedOnceFlowRunsViaExecAndLedgerBecomesPassed(t *testing.T) {
	w := newWorld(t, core, nil)
	out := w.execOut("t-alpha.json")
	if got := w.run(map[string]string{"E2E_EXEC_OUT": out}, execArgs("alpha")...); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	if w.ledgerStatus("alpha") != "passed" {
		t.Fatal("ledger")
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}

func TestAE4ExecRerunsRepeatableRequiresAndReusesPassedOnceRequireOnEveryCall(t *testing.T) {
	w := newWorld(t, core, nil)
	w.seed(map[string]contract.FlowStatus{"alpha": "passed", "beta": "passed", "gamma": "passed"}, map[string]string{"alpha.id": "id-a"})
	for round := 1; round <= 2; round++ {
		w.mu.Lock()
		w.calls = nil
		w.mu.Unlock()
		name := "r" + string(rune('0'+round)) + "-gamma.json"
		if got := w.run(map[string]string{"E2E_EXEC_OUT": w.execOut(name)}, execArgs("gamma")...); got.code != 0 {
			t.Fatalf("round %d exit %d stderr %v", round, got.code, got.stderr)
		}
		if !slices.Equal(w.paths(), []string{"/beta/b1", "/beta/b2", "/beta/b3", "/gamma/g1"}) {
			t.Fatalf("round %d calls %v", round, w.paths())
		}
		result := w.readExec(name)
		if result.Target != "gamma" || !result.Complete || !slices.Equal(stepIDs(result.Steps), []string{"beta/b1", "beta/b2", "beta/b3", "gamma/g1"}) || !slices.Equal(statuses(result.Steps), []string{"passed", "passed", "passed", "passed"}) {
			t.Fatalf("round %d %+v", round, result)
		}
	}
	if w.exists("result.json") || w.exists("manifest.json") {
		t.Fatal("exec touched the full-run files")
	}
}

func TestOnceRequireCaptureMissingFromContextFailsTheStepThatReadsIt(t *testing.T) {
	w := newWorld(t, core, nil)
	w.seed(map[string]contract.FlowStatus{"alpha": "passed"}, nil)
	if got := w.run(map[string]string{"E2E_EXEC_OUT": w.execOut("x.json")}, execArgs("beta")...); got.code != 1 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	if message := w.readExec("x.json").Steps[0].Message; !strings.Contains(message, "alpha.id") {
		t.Fatalf("message %q", message)
	}
}

func TestAE4OnceRequireNotPassedInLedgerExitsTwoAndSendsNothing(t *testing.T) {
	w := newWorld(t, core, nil)
	w.seed(map[string]contract.FlowStatus{"alpha": "failed"}, nil)
	got := w.run(map[string]string{"E2E_EXEC_OUT": w.execOut("x.json")}, execArgs("gamma")...)
	if got.code != 2 || len(w.paths()) != 0 || !strings.Contains(strings.Join(got.stderr, "\n"), "alpha") {
		t.Fatalf("exit %d calls %v stderr %v", got.code, w.paths(), got.stderr)
	}
}

func TestTargetWhoseOnlyRequireIsPassedOnceFlowRunsAlone(t *testing.T) {
	w := newWorld(t, core, nil)
	w.seed(map[string]contract.FlowStatus{"alpha": "passed"}, map[string]string{"alpha.id": "id-a"})
	got := w.run(map[string]string{"E2E_EXEC_OUT": w.execOut("x.json")}, execArgs("beta")...)
	if got.code != 0 || !slices.Equal(w.paths(), []string{"/beta/b1", "/beta/b2", "/beta/b3"}) {
		t.Fatalf("exit %d calls %v stderr %v", got.code, w.paths(), got.stderr)
	}
}

func TestThreeTargetsRunInOrderAndSecondFailureSkipsThirdWithThreeExecFiles(t *testing.T) {
	specs := []flowSpec{
		{ID: "alpha", Stage: "boot", Once: true, Steps: []string{"a1"}},
		{ID: "one", Requires: []string{"alpha"}, Steps: []string{"o1"}},
		{ID: "two", Requires: []string{"alpha"}, Steps: []string{"t1", "t2"}},
		{ID: "three", Requires: []string{"alpha"}, Steps: []string{"h1"}},
	}
	w := newWorld(t, specs, []string{"two/t1"})
	w.seed(map[string]contract.FlowStatus{"alpha": "passed"}, map[string]string{"alpha.id": "id-a"})
	got := w.run(map[string]string{"E2E_EXEC_OUT": w.execOut("20261013T0100-batch.json")}, execArgs("one", "two", "three")...)
	if got.code != 1 || !slices.Equal(w.paths(), []string{"/one/o1", "/two/t1"}) {
		t.Fatalf("exit %d calls %v stderr %v", got.code, w.paths(), got.stderr)
	}
	entries, _ := os.ReadDir(filepath.Join(w.dir, "exec"))
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{"20261013T0100-batch-one.json", "20261013T0100-batch-three.json", "20261013T0100-batch-two.json"}) {
		t.Fatalf("files %v", names)
	}
	one, two, three := w.readExec("20261013T0100-batch-one.json"), w.readExec("20261013T0100-batch-two.json"), w.readExec("20261013T0100-batch-three.json")
	if !slices.Equal(statuses(one.Steps), []string{"passed"}) || !slices.Equal(statuses(two.Steps), []string{"failed", "skipped"}) || !slices.Equal(statuses(three.Steps), []string{"skipped"}) {
		t.Fatalf("%v %v %v", statuses(one.Steps), statuses(two.Steps), statuses(three.Steps))
	}
	if two.StoppedAt == nil || *two.StoppedAt != (contract.StopPoint{Flow: "two", Step: "t1"}) || !one.Complete || !two.Complete || !three.Complete {
		t.Fatalf("%+v", two)
	}
	if w.ledgerStatus("one") != "passed" || w.ledgerStatus("two") != "failed" || w.ledgerStatus("three") != "skipped" {
		t.Fatal("ledger")
	}
	if !slices.Equal(got.stdout, []string{"one › o1 passed 1000ms [200]", "two › t1 failed 1000ms [200]", "two › t2 skipped", "three › h1 skipped"}) {
		t.Fatalf("stdout %q", got.stdout)
	}
}

func TestSkipDoesNotOverwriteEarlierLedgerRecordSinceItOnlyMeansNotStarted(t *testing.T) {
	specs := []flowSpec{
		{ID: "alpha", Stage: "boot", Once: true, Steps: []string{"a1"}},
		{ID: "one", Requires: []string{"alpha"}, Steps: []string{"o1"}},
		{ID: "two", Requires: []string{"alpha"}, Steps: []string{"t1"}},
	}
	w := newWorld(t, specs, []string{"one/o1"})
	w.seed(map[string]contract.FlowStatus{"alpha": "passed", "two": "passed"}, map[string]string{"alpha.id": "id-a"})
	w.run(map[string]string{"E2E_EXEC_OUT": w.execOut("b.json")}, execArgs("one", "two")...)
	if w.ledgerStatus("two") != "passed" {
		t.Fatalf("two = %s", w.ledgerStatus("two"))
	}
}

func TestExecWithoutE2EExecOutWritesTimestampedFlowFileUnderExecDir(t *testing.T) {
	w := newWorld(t, core, nil)
	w.seed(map[string]contract.FlowStatus{"alpha": "passed"}, map[string]string{"alpha.id": "id-a"})
	if got := w.run(nil, execArgs("beta")...); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	entries, _ := os.ReadDir(filepath.Join(w.dir, "exec"))
	if len(entries) != 1 || !regexp.MustCompile(`^[0-9TZ-]+-beta\.json$`).MatchString(entries[0].Name()) {
		t.Fatalf("files %v", entries)
	}
}

package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/exec"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var execMutations = []porttest.Mutation{
	{"exec refuses flows outside the manifest, repeated once flows, unmet prerequisites and stopped environments", "make Denied return nil", "red: the runner starts for nope, seed, orders and a stopped run", "TestExecRefusesUnknownFlowsRepeatedOnceFlowsUnmetPrerequisitesAndStoppedEnvironments"},
	{"exec runs the flow with the pinned runner image", "skip VerifyPin in Execer.Run", "red: no tag check precedes compose run runner", "TestExecRunsTheFlowWithThePinnedImageAndReadsOnlyItsOwnResultFile"},
	{"the exec retag points the pinned ID at the runner tag", "swap the tag arguments in EnsurePinned", "red: the tag call names the tag first", "TestExecRunsTheFlowWithThePinnedImageAndReadsOnlyItsOwnResultFile"},
	{"exec imports the runner again when the pinned image is gone", "skip VerifyPin in Execer.Run", "red: ensures stays at 1 and the runner starts on the wrong image", "TestExecReimportsAGonePinnedImageBeforeTheRunnerAndRefusesAHubThatEmbedsAnotherRunner"},
	{"exec refuses a re-import that brings another runner", "drop the tag comparison after Ensure in EnsurePinned", "red: the runner starts although the hub embeds another runner", "TestExecReimportsAGonePinnedImageBeforeTheRunnerAndRefusesAHubThatEmbedsAnotherRunner"},
}

func TestExecMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "exec_test.go", execMutations)
}

func writeLedger(t *testing.T, w *world, id string, flows map[string]contract.FlowStatus) {
	t.Helper()
	dir, _ := w.hub.Store().RunDir(id)
	l := contract.Ledger{SchemaVersion: contract.RunnerSchemaVersion, Flows: map[string]contract.LedgerEntry{}}
	for f, s := range flows {
		l.Flows[f] = contract.LedgerEntry{Status: s, At: "2026-10-19T00:00:00.000Z"}
	}
	data, _ := json.Marshal(l)
	if err := os.WriteFile(filepath.Join(dir, "work", "e2e", "ledger.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func runnerStarts(w *world) int {
	n := 0
	for _, v := range w.verbs() {
		if v == "compose run runner" {
			n++
		}
	}
	return n
}

func TestExecRefusesUnknownFlowsRepeatedOnceFlowsUnmetPrerequisitesAndStoppedEnvironments(t *testing.T) {
	w := newWorld(t, nil, nil)
	m := w.submit()
	if m.Env != lifecycle.EnvUp {
		t.Fatalf("setup: %+v", m.State)
	}
	writeLedger(t, w, m.ID, map[string]contract.FlowStatus{"seed": contract.FlowStatusPassed, "health": contract.FlowStatusPassed})
	before := runnerStarts(w)
	cases := []struct {
		flow   string
		status int
		says   string
	}{
		{"nope", 404, "no flow nope"},
		{"seed", 409, "once per environment"},
		{"Bad_Flow", 400, "not a flow id"},
	}
	for _, c := range cases {
		r := w.sched.Exec(m.ID, c.flow, scheduler.Actor{By: "tester"})
		if r.Fail == nil || r.Fail.Status != c.status || !strings.Contains(r.Fail.Reason, c.says) {
			t.Errorf("%s: %+v", c.flow, r.Fail)
		}
	}
	writeLedger(t, w, m.ID, map[string]contract.FlowStatus{})
	for _, flow := range []string{"orders", "facts"} {
		r := w.sched.Exec(m.ID, flow, scheduler.Actor{By: "tester"})
		if r.Fail == nil || r.Fail.Status != 409 || !strings.Contains(r.Fail.Reason, "prerequisite seed") {
			t.Errorf("%s: %+v", flow, r.Fail)
		}
	}
	if res := w.sched.Stop(m.ID, scheduler.Actor{By: "tester"}); res.Fail != nil {
		t.Fatal(res.Fail)
	}
	r := w.sched.Exec(m.ID, "health", scheduler.Actor{By: "tester"})
	if r.Fail == nil || r.Fail.Status != 409 || !strings.Contains(r.Fail.Reason, "resume it first") {
		t.Fatalf("stopped: %+v", r.Fail)
	}
	if runnerStarts(w) != before {
		t.Fatalf("a refused exec started the runner (%d -> %d)", before, runnerStarts(w))
	}
}

func TestExecRunsTheFlowWithThePinnedImageAndReadsOnlyItsOwnResultFile(t *testing.T) {
	w := newWorld(t, nil, nil)
	m := w.submit()
	w.fake.TagID = fakedocker.OtherID
	calls := len(w.fake.Calls())
	r := w.sched.Exec(m.ID, "health", scheduler.Actor{By: "tester"})
	if r.Fail != nil {
		t.Fatal(r.Fail)
	}
	view, ok := r.Result.(exec.View)
	if !ok || len(view.Flows) != 4 || view.ExitCode == nil || *view.ExitCode != 0 || view.Reason != "" {
		t.Fatalf("view %+v", r.Result)
	}
	for _, line := range view.Lines {
		if strings.Contains(line, planted) {
			t.Fatalf("exec line %q", line)
		}
	}
	var tagAt, runAt int = -1, -1
	var run []string
	var env []string
	for i, c := range w.fake.Calls()[calls:] {
		switch fakedocker.Verb(c.Args) {
		case "tag":
			tagAt = i
		case "compose run runner":
			runAt, run, env = i, c.Args, c.Env
		}
	}
	if tagAt < 0 || runAt < tagAt {
		t.Fatalf("tag at %d, runner at %d", tagAt, runAt)
	}
	if tag := w.fake.Calls()[calls+tagAt].Args; !slices.Equal(tag, []string{"tag", fakedocker.PinnedID, fakedocker.RunnerTag}) {
		t.Fatalf("tag call %q, want the pinned ID pointed back at the runner tag", tag)
	}
	if !slices.Equal(run[len(run)-4:], []string{"run", "smoke", "--flow", "health"}) {
		t.Fatalf("runner args %q", run)
	}
	if len(env) != 1 || !strings.HasPrefix(env[0], "E2E_EXEC_OUT=/work/e2e/exec/") || !strings.HasSuffix(env[0], "-health.json") {
		t.Fatalf("runner env %q", env)
	}
}

func TestExecReimportsAGonePinnedImageBeforeTheRunnerAndRefusesAHubThatEmbedsAnotherRunner(t *testing.T) {
	w := newWorld(t, nil, nil)
	m := w.submit()
	w.fake.TagID, w.fake.PinnedGone = fakedocker.OtherID, true
	calls := len(w.fake.Calls())
	if r := w.sched.Exec(m.ID, "health", scheduler.Actor{By: "tester"}); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	since := w.fake.Calls()[calls:]
	runAt := slices.IndexFunc(since, func(c fakedocker.Call) bool { return fakedocker.Verb(c.Args) == "compose run runner" })
	if w.ensures != 2 || runAt < 0 || w.ensureAt[1] > calls+runAt {
		t.Fatalf("re-import: ensures %d at %v, runner at %d", w.ensures, w.ensureAt, calls+runAt)
	}
	if slices.ContainsFunc(since, func(c fakedocker.Call) bool { return fakedocker.Verb(c.Args) == "tag" }) {
		t.Fatal("a gone pinned image was retagged instead of imported")
	}
	if pin := w.meta(m.ID).RunnerImage; pin == nil || pin.Tag != fakedocker.RunnerTag || pin.ID != fakedocker.PinnedID {
		t.Fatalf("pin after re-import %+v", pin)
	}
	w.mu.Lock()
	w.ensureTag = "cockpit-runner:ffffffffffff"
	w.mu.Unlock()
	before := runnerStarts(w)
	r := w.sched.Exec(m.ID, "health", scheduler.Actor{By: "tester"})
	if r.Fail == nil || !strings.Contains(r.Fail.Reason, "this hub embeds cockpit-runner:ffffffffffff instead") {
		t.Fatalf("exec with another embedded runner: %+v", r)
	}
	if runnerStarts(w) != before {
		t.Fatalf("the runner started on another embedded runner (%d -> %d)", before, runnerStarts(w))
	}
}

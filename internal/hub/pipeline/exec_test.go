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
	if !ok || len(view.Flows) != 2 || view.ExitCode == nil || *view.ExitCode != 0 || view.Reason != "" {
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
	if !slices.Equal(run[len(run)-4:], []string{"run", "smoke", "--flow", "health"}) {
		t.Fatalf("runner args %q", run)
	}
	if len(env) != 1 || !strings.HasPrefix(env[0], "E2E_EXEC_OUT=/work/e2e/exec/") || !strings.HasSuffix(env[0], "-health.json") {
		t.Fatalf("runner env %q", env)
	}
}

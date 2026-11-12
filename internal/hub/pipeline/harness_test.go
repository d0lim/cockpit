package pipeline

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var harnessMutations = []porttest.Mutation{
	{"the prepare stage records the harness version", "drop HarnessVersion from the ImageKeys patch in prepare", "red: the run records harness version 0", "TestAPipelineRunRecordsTheCurrentHarnessVersionAndItsKeptEnvironmentResumesAndExecs"},
	{"exec refuses a kept run of another harness", "drop the otherHarness check in Exec", "red: the runner starts against the old harness", "TestAKeptRunOfAnOlderHarnessIsRefusedForResumeAndExecWithBothVersionsWhileStopAndDestroyWork"},
	{"resume refuses a kept run of another harness before its state changes", "drop the otherHarness check in Resume", "red: the run passes through resuming and gets the notice from envops", "TestAKeptRunOfAnOlderHarnessIsRefusedForResumeAndExecWithBothVersionsWhileStopAndDestroyWork"},
	{"a resume that reaches envops for another harness closes as failed", "drop the harness check in Ops.Resume", "red: the redo succeeds and starts the old harness", "TestAResumeOfAnOlderHarnessRunThatReachesTheEnvironmentClosesAsFailedWithTheNotice"},
	{"arming skips a run of another harness", "drop the harness check before arming in Ops.Transition", "red: the collector of the old harness run is armed", "TestAReconciledUpRunOfAnOlderHarnessArmsNoCollectorWhileACurrentOneDoes"},
}

func TestHarnessMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "harness_test.go", harnessMutations)
}

var tester = scheduler.Actor{By: "tester"}

func olderHarness(t *testing.T, w *world, id string) {
	t.Helper()
	if _, err := w.hub.Store().PatchMeta(id, func(m *store.Meta) { m.HarnessVersion = 0 }); err != nil {
		t.Fatal(err)
	}
}

func assertHarnessNotice(t *testing.T, what string, f *ticker.Failure) {
	t.Helper()
	if f == nil || f.Status != 409 || f.Code != ticker.CodeConflictState {
		t.Fatalf("%s: %+v", what, f)
	}
	for _, want := range []string{"harness version 7 or earlier", "harness version 8", "destroy the run and submit again"} {
		if !strings.Contains(f.Reason, want) {
			t.Fatalf("%s: %q lacks %q", what, f.Reason, want)
		}
	}
}

func verbsSince(w *world, calls int) []string {
	var out []string
	for _, c := range w.fake.Calls()[calls:] {
		out = append(out, fakedocker.Verb(c.Args))
	}
	return out
}

func TestAPipelineRunRecordsTheCurrentHarnessVersionAndItsKeptEnvironmentResumesAndExecs(t *testing.T) {
	w := newWorld(t, nil, nil)
	m := w.submit()
	if m.Env != lifecycle.EnvUp || m.HarnessVersion != compose.HarnessVersion || compose.HarnessVersion != 8 {
		t.Fatalf("state %+v harness %d", m.State, m.HarnessVersion)
	}
	before := runnerStarts(w)
	if r := w.sched.Exec(m.ID, "health", tester); r.Fail != nil || runnerStarts(w) != before+1 {
		t.Fatalf("exec: %+v, runner starts %d", r.Fail, runnerStarts(w)-before)
	}
	if r := w.sched.Stop(m.ID, tester); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	if r := w.sched.Resume(m.ID, tester); r.Fail != nil || r.Run.Env != lifecycle.EnvUp {
		t.Fatalf("resume: %+v", r.Fail)
	}
}

func TestAKeptRunOfAnOlderHarnessIsRefusedForResumeAndExecWithBothVersionsWhileStopAndDestroyWork(t *testing.T) {
	w := newWorld(t, nil, nil)
	m := w.submit()
	olderHarness(t, w, m.ID)
	calls := len(w.fake.Calls())
	assertHarnessNotice(t, "exec", w.sched.Exec(m.ID, "health", tester).Fail)
	if got := verbsSince(w, calls); len(got) != 0 {
		t.Fatalf("exec called docker: %v", got)
	}
	if r := w.sched.Stop(m.ID, tester); r.Fail != nil || r.Run.Env != lifecycle.EnvStopped {
		t.Fatalf("stop: %+v", r.Fail)
	}
	calls = len(w.fake.Calls())
	assertHarnessNotice(t, "resume", w.sched.Resume(m.ID, tester).Fail)
	if got := verbsSince(w, calls); len(got) != 0 {
		t.Fatalf("resume called docker: %v", got)
	}
	if after := w.meta(m.ID); after.Phase != lifecycle.Done || after.Env != lifecycle.EnvStopped || after.Notice != "" {
		t.Fatalf("resume changed the run: %+v notice %q", after.State, after.Notice)
	}
	r := w.sched.Destroy(m.ID, tester)
	if r.Fail != nil || w.meta(m.ID).Env != lifecycle.EnvDestroyed || !slices.Contains(verbsSince(w, calls), "compose down") {
		t.Fatalf("destroy: %+v verbs %v", r.Fail, verbsSince(w, calls))
	}
}

func TestAResumeOfAnOlderHarnessRunThatReachesTheEnvironmentClosesAsFailedWithTheNotice(t *testing.T) {
	w := newWorld(t, nil, nil)
	m := w.submit()
	if r := w.sched.Stop(m.ID, tester); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	olderHarness(t, w, m.ID)
	if _, err := w.hub.Store().ApplyEvent(m.ID, lifecycle.Resume, lifecycle.Detail{}, nil); err != nil {
		t.Fatal(err)
	}
	calls := len(w.fake.Calls())
	if err := w.hub.ops.Redo(context.Background(), m.ID); err == nil {
		t.Fatal("redo of a resume of an older harness run succeeded")
	}
	if got := verbsSince(w, calls); slices.Contains(got, "start") || slices.Contains(got, "compose up") {
		t.Fatalf("the old harness was started: %v", got)
	}
	after := w.meta(m.ID)
	if after.Phase != lifecycle.Done || after.Env != lifecycle.EnvStopped || !strings.Contains(after.Notice, "harness version 7 or earlier") {
		t.Fatalf("after redo %+v notice %q", after.State, after.Notice)
	}
}

func reconciledUp(w *world, id string) {
	stopped := w.meta(id)
	up := stopped
	up.Env = lifecycle.EnvUp
	w.hub.ops.Transition(store.Transition{Before: stopped.State, Event: lifecycle.Observed, Detail: lifecycle.Detail{Observation: lifecycle.ObservedUp}, After: up})
}

func TestAReconciledUpRunOfAnOlderHarnessArmsNoCollectorWhileACurrentOneDoes(t *testing.T) {
	w := collectingWorld(t)
	m := w.submit()
	if r := w.sched.Stop(m.ID, tester); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	olderHarness(t, w, m.ID)
	reconciledUp(w, m.ID)
	if states := w.hub.ops.Collect.Status(m.ID); len(states) != 1 || states[0].State == collect.StateArmed {
		t.Fatalf("older harness: collector states %+v", states)
	}
	if _, err := w.hub.Store().PatchMeta(m.ID, func(x *store.Meta) { x.HarnessVersion = compose.HarnessVersion }); err != nil {
		t.Fatal(err)
	}
	reconciledUp(w, m.ID)
	if states := w.hub.ops.Collect.Status(m.ID); len(states) != 1 || states[0].State != collect.StateArmed {
		t.Fatalf("canary: current harness collector states %+v", states)
	}
}

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/report"
)

var abortMutations = []porttest.Mutation{
	{"an abort while preparing releases the lease", "skip CloseWithoutEnv in settle", "red: the lease outlives the aborted run", "TestAnAbortWhilePreparingClosesAsAbortedWithoutAnEnvironmentAndReleasesTheLease"},
	{"an abort during the boot wait stops the environment", "skip StopQuietly in settle", "red: compose stop never runs", "TestAnAbortWhileWaitingForTheBootCutsTheWaitAndKeepsTheEnvironmentStopped"},
	{"the destroy of a running run brings it down once and releases the lease", "skip releaseLease in envops.Destroy", "red: the lease outlives the destroy", "TestADestroyWhileTheRunnerRunsAbortsThePipelineAndOnlyTheDestroyBringsTheProjectDown"},
}

func TestAbortMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "abort_test.go", abortMutations)
}

func (w *world) startInBackground() (string, chan struct{}) {
	w.t.Helper()
	res := w.sched.Submit(context.Background(), scheduler.SubmitInput{Worktree: w.repo, Submitter: "tester"})
	if res.Fail != nil {
		w.t.Fatalf("submit: %+v", res.Fail)
	}
	done := make(chan struct{})
	go func() {
		w.sched.Tick()
		w.sched.Idle()
		close(done)
	}()
	return res.ID, done
}

func finish(t *testing.T, done chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("the pipeline did not finish")
	}
}

func TestAnAbortWhilePreparingClosesAsAbortedWithoutAnEnvironmentAndReleasesTheLease(t *testing.T) {
	w := newWorld(t, nil, nil)
	entered := make(chan struct{})
	w.onEnsure = func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}
	id, done := w.startInBackground()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the pipeline never reached the runner import")
	}
	if r := w.sched.Abort(id, scheduler.Actor{By: "tester"}); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	finish(t, done)
	m := w.meta(id)
	closedState(t, m, lifecycle.VerdictAborted, lifecycle.EnvAbsent, lifecycle.Preparing)
	if w.leased(id) || indexOf(w.verbs(), "compose up") >= 0 {
		t.Fatalf("leased %v verbs %v", w.leased(id), w.verbs())
	}
}

func TestAnAbortWhileWaitingForTheBootCutsTheWaitAndKeepsTheEnvironmentStopped(t *testing.T) {
	w := newWorldWith(t, func(o *Options) { o.Limits.Boot = time.Minute }, func(f *fakedocker.Fake) { f.Unhealthy = true }, nil)
	id, done := w.startInBackground()
	waitFor(t, "the boot wait", func() bool { return indexOf(w.verbs(), "compose ps") >= 0 && w.meta(id).Phase == lifecycle.Booting })
	if r := w.sched.Abort(id, scheduler.Actor{By: "tester"}); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	finish(t, done)
	m := w.meta(id)
	closedState(t, m, lifecycle.VerdictAborted, lifecycle.EnvStopped, lifecycle.Booting)
	dir, _ := w.hub.Store().RunDir(id)
	if _, err := os.Stat(filepath.Join(dir, report.DirReport)); !os.IsNotExist(err) {
		t.Fatalf("an aborted boot wrote a report: %v", err)
	}
	if indexOf(w.verbs(), "compose stop") < 0 || indexOf(w.verbs(), "compose run runner") >= 0 || !w.leased(id) {
		t.Fatalf("leased %v verbs %v", w.leased(id), w.verbs())
	}
}

func TestADestroyWhileTheRunnerRunsAbortsThePipelineAndOnlyTheDestroyBringsTheProjectDown(t *testing.T) {
	w := newWorldWith(t, func(o *Options) { o.Limits.Suite = time.Minute }, func(f *fakedocker.Fake) { f.RunnerHangs = true }, nil)
	id, done := w.startInBackground()
	waitFor(t, "the runner wait", func() bool { return indexOf(w.verbs(), "wait") >= 0 })
	if r := w.sched.Destroy(id, scheduler.Actor{By: "tester"}); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	finish(t, done)
	w.sched.Idle()
	m := w.meta(id)
	if m.Phase != lifecycle.Done || m.Env != lifecycle.EnvDestroyed || m.Verdict != lifecycle.VerdictAborted {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	downs := 0
	for _, v := range w.verbs() {
		if v == "compose down" {
			downs++
		}
	}
	if downs != 1 || w.leased(id) {
		t.Fatalf("compose down ran %d times, leased %v: %v", downs, w.leased(id), w.verbs())
	}
}

package pipeline

import (
	"slices"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/hub/exec"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var execClaimMutations = []porttest.Mutation{
	{"one claim per run", "let Core.claim count a second claim instead of refusing it", "red: the second exec starts a second runner", "TestTwoConcurrentExecsRunOnceAndTheSecondIsRefusedWhileAbortEndsTheFirst"},
	{"exec claims the run before it checks and runs", "check Denied and run the exec outside RunOp", "red: the second exec passes the check and starts a second runner", "TestTwoConcurrentExecsRunOnceAndTheSecondIsRefusedWhileAbortEndsTheFirst"},
	{"abort ends a running exec", "keep Abort for the active pipeline run only", "red: abort during an exec is refused and the runner keeps waiting", "TestTwoConcurrentExecsRunOnceAndTheSecondIsRefusedWhileAbortEndsTheFirst"},
	{"a run with a running exec offers only abort", "leave AllowedEvents to the lifecycle table during an exec", "red: stop, destroy and keep are offered while the exec holds the run", "TestStopDuringARunningExecIsRefusedAndRunsNoDockerStopUntilTheExecEnds"},
	{"stop is refused while an exec holds the run", "let Operate claim a run that is already claimed", "red: compose stop runs under the exec", "TestStopDuringARunningExecIsRefusedAndRunsNoDockerStopUntilTheExecEnds"},
}

func TestExecClaimMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "execclaim_test.go", execClaimMutations)
}

func hangingExecWorld(t *testing.T) (*world, string) {
	w := newWorldWith(t, func(o *Options) { o.Limits.Suite = time.Minute }, nil, nil)
	m := w.submit()
	if m.Verdict != lifecycle.VerdictPassed || m.Env != lifecycle.EnvUp {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	w.fake.RunnerHangs = true
	return w, m.ID
}

func (w *world) startExec(id string) (int, chan scheduler.ExecResponse) {
	w.t.Helper()
	before := runnerStarts(w)
	done := make(chan scheduler.ExecResponse, 1)
	go func() { done <- w.sched.Exec(id, "health", scheduler.Actor{By: "tester"}) }()
	waitFor(w.t, "the exec runner waiting", func() bool {
		return runnerStarts(w) == before+1 && slices.Contains(w.verbs()[len(w.verbs())-1:], "wait")
	})
	return before, done
}

func busyRefusal(t *testing.T, what string, fail *ticker.Failure) {
	t.Helper()
	if fail == nil || fail.Status != 409 || fail.Code != ticker.CodeConflictState || fail.Reason != ticker.ReasonBusy {
		t.Fatalf("%s while an exec runs: %+v", what, fail)
	}
}

func TestTwoConcurrentExecsRunOnceAndTheSecondIsRefusedWhileAbortEndsTheFirst(t *testing.T) {
	w, id := hangingExecWorld(t)
	before, first := w.startExec(id)
	second := w.sched.Exec(id, "health", scheduler.Actor{By: "tester"})
	busyRefusal(t, "a second exec", second.Fail)
	if r := w.sched.Abort(id, scheduler.Actor{By: "tester"}); r.Fail != nil {
		t.Fatalf("abort during an exec: %+v", r.Fail)
	}
	var resp scheduler.ExecResponse
	select {
	case resp = <-first:
	case <-time.After(10 * time.Second):
		t.Fatal("the aborted exec did not return")
	}
	view, ok := resp.Result.(exec.View)
	if resp.Fail != nil || !ok || view.Reason != "the exec was aborted" {
		t.Fatalf("aborted exec %+v", resp)
	}
	if got := runnerStarts(w); got != before+1 {
		t.Fatalf("runner started %d times for two execs", got-before)
	}
	forced := slices.ContainsFunc(w.fake.Calls(), func(c fakedocker.Call) bool {
		return slices.Equal(c.Args, []string{"rm", "-f", fakedocker.ContainerID})
	})
	if !forced {
		t.Fatalf("the aborted exec runner was not force-removed: %v", w.verbs())
	}
}

func TestStopDuringARunningExecIsRefusedAndRunsNoDockerStopUntilTheExecEnds(t *testing.T) {
	w, id := hangingExecWorld(t)
	_, first := w.startExec(id)
	if got := w.sched.AllowedEvents(w.meta(id)); !slices.Equal(got, []lifecycle.Event{lifecycle.Abort}) {
		t.Fatalf("allowed events during an exec %v, want only abort", got)
	}
	busyRefusal(t, "stop", w.sched.Stop(id, scheduler.Actor{By: "tester"}).Fail)
	busyRefusal(t, "destroy", w.sched.Destroy(id, scheduler.Actor{By: "tester"}).Fail)
	if indexOf(w.verbs(), "compose stop") >= 0 || indexOf(w.verbs(), "compose down") >= 0 {
		t.Fatalf("docker stop or down ran under the exec: %v", w.verbs())
	}
	if m := w.meta(id); m.Env != lifecycle.EnvUp || m.Phase != lifecycle.Done {
		t.Fatalf("the refused stop moved the run to %+v", m.State)
	}
	w.sched.Abort(id, scheduler.Actor{By: "tester"})
	<-first
	if got := w.sched.AllowedEvents(w.meta(id)); !slices.Contains(got, lifecycle.Stop) || slices.Contains(got, lifecycle.Abort) {
		t.Fatalf("allowed events after the exec %v", got)
	}
	if r := w.sched.Stop(id, scheduler.Actor{By: "tester"}); r.Fail != nil {
		t.Fatalf("stop after the exec ended: %+v", r.Fail)
	}
	if indexOf(w.verbs(), "compose stop") < 0 || w.meta(id).Env != lifecycle.EnvStopped {
		t.Fatalf("the stop after the exec did not run: %v", w.verbs())
	}
}

package scheduler

import (
	"slices"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

func TestFinishedRunIsStoppedRightAwayAndTheQueuedRunStartsAfterThatStop(t *testing.T) {
	s, w := setup(t, nil)
	a := s.Submit(ctx, sub("c1")).ID
	s.Tick()
	b := s.Submit(ctx, sub("c2")).ID
	w.finish(a, 30)
	until(t, func() bool { return len(w.startedRuns()) == 2 && read(t, w, a).Env == lifecycle.EnvStopped })
	if !slices.Equal(suffixed(w.snapshot(), " stop"), []string{a + " stop"}) || !inOrder(w.snapshot(), a+" stop", b+" pipeline") {
		t.Fatalf("%v", w.snapshot())
	}
	if m := read(t, w, a); m.Phase != lifecycle.Done || m.Verdict != lifecycle.VerdictPassed || m.StopReason != lifecycle.StopDone {
		t.Fatalf("%+v", m)
	}
	list, _ := s.List()
	if list.Up != (Count{Count: 1, Cap: 3}) {
		t.Fatalf("%+v", list.Up)
	}
}

func TestResumedEnvironmentIsNotStoppedAgain(t *testing.T) {
	s, w := setup(t, nil)
	a := s.Submit(ctx, sub("c1")).ID
	s.Tick()
	w.finish(a, 30)
	until(t, func() bool { return read(t, w, a).Env == lifecycle.EnvStopped })
	s.Idle()
	if r := s.Resume(a, Actor{By: "cli"}); !r.OK() {
		t.Fatal(r.Fail)
	}
	s.Idle()
	if m := read(t, w, a); m.Env != lifecycle.EnvUp || m.StopReason != lifecycle.StopNone {
		t.Fatalf("%+v", m)
	}
	if got := suffixed(w.snapshot(), " stop"); !slices.Equal(got, []string{a + " stop"}) {
		t.Fatalf("%v", got)
	}
}

func TestAbortedRunIsAlsoStoppedAndKeepsTheAbortedVerdict(t *testing.T) {
	s, w := setup(t, nil)
	a := s.Submit(ctx, sub("c1")).ID
	s.Tick()
	if r := s.Abort(a, Actor{By: "cli"}); !r.OK() {
		t.Fatal(r.Fail)
	}
	until(t, func() bool { return read(t, w, a).Env == lifecycle.EnvStopped })
	s.Idle()
	if m := read(t, w, a); m.Verdict != lifecycle.VerdictAborted || m.StopReason != lifecycle.StopDone {
		t.Fatalf("%+v", m)
	}
}

func TestStopOnDoneOffLeavesTheFinishedRunUp(t *testing.T) {
	s, w := setup(t, nil, func(d *Deps) { d.StopOnDone = false })
	a := s.Submit(ctx, sub("c1")).ID
	s.Tick()
	w.finish(a, 30)
	until(t, func() bool { return read(t, w, a).Phase == lifecycle.Done })
	s.Idle()
	if m := read(t, w, a); m.Env != lifecycle.EnvUp || m.StopReason != lifecycle.StopNone || len(suffixed(w.snapshot(), " stop")) != 0 {
		t.Fatalf("%+v %v", m, w.snapshot())
	}
}

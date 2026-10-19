package scheduler

import (
	"slices"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/pool"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
)

func upCap(n int) func(*Deps) { return func(d *Deps) { d.Limits.UpCap = n } }

func TestSixthFinishedRunDestroysOnlyTheOldestUnkeptAndKeepsTheJustFinishedOne(t *testing.T) {
	kept5 := []store.Meta{preserved(1, 10, kept), preserved(2, 11), preserved(3, 12, kept), preserved(4, 13), preserved(5, 14)}
	s, w := setup(t, kept5, upCap(10))
	sixth := s.Submit(ctx, sub("c6")).ID
	s.Tick()
	if !slices.Equal(w.startedRuns(), []string{sixth}) {
		t.Fatalf("%v", w.startedRuns())
	}
	w.finish(sixth, 20)
	until(t, func() bool { return read(t, w, rid(2)).Env == lifecycle.EnvDestroyed })
	until(t, func() bool { return read(t, w, sixth).Env == lifecycle.EnvStopped })
	s.Idle()
	if got := suffixed(w.snapshot(), " down"); !slices.Equal(got, []string{rid(2) + " down"}) {
		t.Fatalf("%v", got)
	}
	if read(t, w, rid(2)).DestroyReason != lifecycle.DestroyPool {
		t.Fatal("destroy reason")
	}
	for _, id := range []string{rid(1), rid(3), rid(4), rid(5)} {
		if read(t, w, id).Env != lifecycle.EnvUp {
			t.Fatalf("%s touched", id)
		}
	}
	if m := read(t, w, sixth); m.Phase != lifecycle.Done || m.StopReason != lifecycle.StopDone {
		t.Fatalf("%+v", m)
	}
	list, _ := s.List()
	if list.Pool != (Size{Size: 5, Max: 5}) {
		t.Fatalf("%+v", list.Pool)
	}
}

func TestAllFiveKeptMakesANewRunWaitWithAReasonAndUnkeepingOneStartsIt(t *testing.T) {
	var all []store.Meta
	for n := 1; n <= 5; n++ {
		all = append(all, preserved(n, 10+n, kept))
	}
	s, w := setup(t, all, upCap(10))
	id := s.Submit(ctx, sub("c6")).ID
	s.Tick()
	if len(w.startedRuns()) != 0 || s.WaitReasonOf(id) != lifecycle.WaitPoolAllKept {
		t.Fatalf("%v %q", w.startedRuns(), s.WaitReasonOf(id))
	}
	if r := s.Unkeep(rid(3), Actor{By: "cli"}); !r.OK() {
		t.Fatalf("%+v", r.Fail)
	}
	s.Tick()
	if !slices.Equal(w.startedRuns(), []string{id}) {
		t.Fatalf("%v", w.startedRuns())
	}
}

func TestKeepFillingThePoolDuringARunLeavesSixWithoutDestroyingUntilAPersonDestroysOne(t *testing.T) {
	var runs []store.Meta
	for n := 1; n <= 4; n++ {
		runs = append(runs, preserved(n, 10+n, kept))
	}
	s, w := setup(t, append(runs, preserved(5, 15)), upCap(10))
	a := s.Submit(ctx, sub("c6")).ID
	s.Tick()
	if r := s.Keep(rid(5), Actor{By: "cli"}); !r.OK() {
		t.Fatal(r.Fail)
	}
	b := s.Submit(ctx, sub("c7")).ID
	w.finish(a, 30)
	until(t, func() bool { return s.WaitReasonOf(b) == lifecycle.WaitPoolOver })
	list, _ := s.List()
	if len(suffixed(w.snapshot(), " down")) != 0 || list.Pool.Size != 6 || !slices.Equal(w.startedRuns(), []string{a}) {
		t.Fatalf("%v %d %v", w.snapshot(), list.Pool.Size, w.startedRuns())
	}
	if r := s.Destroy(rid(1), Actor{By: "cli"}); !r.OK() {
		t.Fatal(r.Fail)
	}
	until(t, func() bool { return len(w.startedRuns()) == 2 })
	if !slices.Equal(w.startedRuns(), []string{a, b}) {
		t.Fatalf("%v", w.startedRuns())
	}
}

func TestStartableHeadStopsTheOldestLastUpKeptRunFirstAndItsResumeIsRefusedWithCandidates(t *testing.T) {
	s, w := setup(t, []store.Meta{preserved(1, 30), preserved(2, 10), preserved(3, 20)})
	id := s.Submit(ctx, sub("c4")).ID
	s.Tick()
	if calls := w.snapshot(); !inOrder(calls, rid(2)+" stop", "leaseBlock") || !slices.Equal(suffixed(calls, " stop"), []string{rid(2) + " stop"}) || len(suffixed(calls, "leaseBlock")) != 1 {
		t.Fatalf("%v", calls)
	}
	if head := read(t, w, id); head.Phase != lifecycle.Preparing || head.Slot == nil {
		t.Fatalf("the tick did not start the head after the stop: %+v", head)
	}
	if m := read(t, w, rid(2)); m.Env != lifecycle.EnvStopped || m.StopReason != lifecycle.StopCap {
		t.Fatalf("%+v", m)
	}
	r := s.Resume(rid(2), Actor{By: "cli"})
	if r.Fail == nil || r.Fail.Status != 409 || r.Fail.Code != ticker.CodeConflictCap {
		t.Fatalf("%+v", r)
	}
	var ids []string
	for _, c := range r.Fail.Candidates {
		ids = append(ids, c.ID)
	}
	if !slices.Equal(ids, []string{rid(3), rid(1)}) || s.CandidatesForResume()[0].ID != rid(3) || read(t, w, rid(2)).Env != lifecycle.EnvStopped {
		t.Fatalf("%v", ids)
	}
}

func TestRunningForeignComposeProjectTakesOneSlotSoTheOlderOfTwoKeptRunsStops(t *testing.T) {
	s, w := setup(t, []store.Meta{preserved(1, 30), preserved(2, 10)})
	w.foreign = []reconcile.Project{{Name: "toy-shop", Status: "running(3)"}}
	s.Submit(ctx, sub("c3"))
	s.Tick()
	if got := suffixed(w.snapshot(), " stop"); !slices.Equal(got, []string{rid(2) + " stop"}) {
		t.Fatalf("%v", got)
	}
	list, _ := s.List()
	if read(t, w, rid(1)).Env != lifecycle.EnvUp || list.Up != (Count{Count: 3, Cap: 3}) {
		t.Fatalf("%+v", list.Up)
	}
}

func TestResumeWithinTheCapStartsTheContainersAndRecordsANewLastUp(t *testing.T) {
	s, w := setup(t, []store.Meta{preserved(1, 10, func(m *store.Meta) { m.Env = lifecycle.EnvStopped; m.StopReason = lifecycle.StopPerson })})
	if r := s.Resume(rid(1), Actor{By: "cli"}); !r.OK() {
		t.Fatal(r.Fail)
	}
	m := read(t, w, rid(1))
	if m.Phase != lifecycle.Done || m.Env != lifecycle.EnvUp || m.StopReason != lifecycle.StopNone || m.LastUpAt == at(10) {
		t.Fatalf("%+v", m)
	}
	if !inOrder(w.snapshot(), rid(1)+" start", "awaitBoot") {
		t.Fatalf("%v", w.snapshot())
	}
}

func TestSchedulerUsesTheConfiguredLimitsInsteadOfTheDefaults(t *testing.T) {
	limits := pool.Limits{PoolMax: 1, QueueMax: 1, UpCap: 10}
	s, w := setup(t, []store.Meta{preserved(1, 10)}, func(d *Deps) { d.Limits = limits })
	first := s.Submit(ctx, sub("c1"))
	if r := s.Submit(ctx, sub("c2")); r.Fail == nil || r.Fail.Code != ticker.CodeQueueFull {
		t.Fatalf("%+v", r)
	}
	s.Tick()
	w.finish(first.ID, 20)
	until(t, func() bool { return read(t, w, rid(1)).Env == lifecycle.EnvDestroyed })
	list, _ := s.List()
	if list.Pool.Max != 1 || list.Up.Cap != 10 {
		t.Fatalf("%+v", list)
	}
}

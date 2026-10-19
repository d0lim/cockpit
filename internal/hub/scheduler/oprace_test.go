package scheduler

import (
	"sync"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var opRaceMutations = []porttest.Mutation{
	{"overlapping operations on one run keep it busy until the last one ends", "delete the run's mark in Core.release whatever its count", "red: the refused stop clears the busy mark of the destroy still running", "TestAStopRefusedWhileADestroyRunsLeavesTheRunBusyUntilTheDestroyEnds"},
	{"an operation is busy before its event lands", "apply the event before Core.Operate claims the run", "red: Busy is false when the event lands and the tick's stalled-operation redo runs the operation a second time", "TestATickLandingAsAStopResumeOrDestroyEventIsAppliedFindsTheOperationBusyAndDoesNotRunItAgain"},
}

func TestOpRaceMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "oprace_test.go", opRaceMutations)
}

func TestATickLandingAsAStopResumeOrDestroyEventIsAppliedFindsTheOperationBusyAndDoesNotRunItAgain(t *testing.T) {
	stopped := func(m *store.Meta) { m.Env = lifecycle.EnvStopped }
	cases := []struct {
		name  string
		start func(*store.Meta)
		event lifecycle.Event
		op    func(s *Scheduler, id string) ticker.Result
		call  string
		after lifecycle.Env
	}{
		{"stop", nil, lifecycle.Stop, func(s *Scheduler, id string) ticker.Result { return s.Stop(id, Actor{By: "cli"}) }, " stop", lifecycle.EnvStopped},
		{"resume", stopped, lifecycle.Resume, func(s *Scheduler, id string) ticker.Result { return s.Resume(id, Actor{By: "cli"}) }, " start", lifecycle.EnvUp},
		{"destroy", nil, lifecycle.Destroy, func(s *Scheduler, id string) ticker.Result { return s.Destroy(id, Actor{By: "cli"}) }, " down", lifecycle.EnvDestroyed},
	}
	evaluated := 0
	for _, c := range cases {
		var edits []func(*store.Meta)
		if c.start != nil {
			edits = append(edits, c.start)
		}
		s, w := setup(t, []store.Meta{preserved(1, 30, edits...)})
		id := rid(1)
		var mu sync.Mutex
		landed, busy := 0, false
		unobserve := store.Observe(w.store.Home(), func(tr store.Transition) {
			if tr.After.ID != id || tr.Event != c.event {
				return
			}
			mu.Lock()
			landed++
			busy = s.core.Busy(id)
			mu.Unlock()
			s.Tick()
		})
		r := c.op(s, id)
		s.Idle()
		unobserve()
		mu.Lock()
		gotLanded, gotBusy := landed, busy
		mu.Unlock()
		if r.Fail != nil {
			t.Errorf("%s: %+v", c.name, r.Fail)
		}
		if gotLanded != 1 || !gotBusy {
			t.Errorf("%s: the event landed %d times, busy %v", c.name, gotLanded, gotBusy)
		}
		if calls := suffixed(w.snapshot(), c.call); len(calls) != 1 {
			t.Errorf("%s: %v ran %d times: %v", c.name, c.call, len(calls), w.snapshot())
		}
		if m := read(t, w, id); m.Env != c.after || m.Phase != lifecycle.Done {
			t.Errorf("%s: state %+v", c.name, m.State)
		}
		evaluated++
	}
	if evaluated != 3 {
		t.Fatalf("evaluated %d", evaluated)
	}
}

func TestAStopRefusedWhileADestroyRunsLeavesTheRunBusyUntilTheDestroyEnds(t *testing.T) {
	s, w := setup(t, []store.Meta{preserved(1, 30)})
	id := rid(1)
	var refused ticker.Result
	busyAfter := false
	unobserve := store.Observe(w.store.Home(), func(tr store.Transition) {
		if tr.After.ID != id || tr.Event != lifecycle.Destroy {
			return
		}
		refused = s.Stop(id, Actor{By: "cli"})
		busyAfter = s.core.Busy(id)
	})
	r := s.Destroy(id, Actor{By: "cli"})
	s.Idle()
	unobserve()
	if r.Fail != nil {
		t.Fatal(r.Fail)
	}
	if refused.Fail == nil || refused.Fail.Status != 409 {
		t.Fatalf("a stop during the destroy was not refused: %+v", refused)
	}
	if !busyAfter {
		t.Fatal("the refused stop cleared the busy mark of the destroy still running")
	}
	if m := read(t, w, id); m.Env != lifecycle.EnvDestroyed || len(suffixed(w.snapshot(), " down")) != 1 || len(suffixed(w.snapshot(), " stop")) != 0 {
		t.Fatalf("state %+v calls %v", m.State, w.snapshot())
	}
}

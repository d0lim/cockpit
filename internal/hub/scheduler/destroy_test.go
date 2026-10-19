package scheduler

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
)

func calledFor(calls []string, id string) []string {
	var out []string
	for _, c := range calls {
		if strings.HasPrefix(c, id+" ") {
			out = append(out, c)
		}
	}
	return out
}

func TestDestroyingAQueuedRunRemovesItFromTheQueueWithoutDocker(t *testing.T) {
	s, w := setup(t, nil)
	s.Submit(ctx, sub("c1"))
	s.Tick()
	queued := s.Submit(ctx, sub("c2")).ID
	if r := s.Destroy(queued, Actor{By: "cli"}); !r.OK() {
		t.Fatal(r.Fail)
	}
	m := read(t, w, queued)
	if m.Phase != lifecycle.Done || m.Env != lifecycle.EnvDestroyed || m.Verdict != lifecycle.VerdictAborted || m.DestroyReason != lifecycle.DestroyPerson {
		t.Fatalf("%+v", m)
	}
	if q := w.store.ReadState().State.Queue; len(q) != 0 || len(calledFor(w.snapshot(), queued)) != 0 {
		t.Fatalf("%v %v", q, w.snapshot())
	}
}

func TestDestroyingTheActiveRunAbortsItTakesItDownAndReleasesTheLease(t *testing.T) {
	s, w := setup(t, nil)
	active := s.Submit(ctx, sub("c1")).ID
	s.Tick()
	if r := s.Destroy(active, Actor{By: "cli"}); !r.OK() {
		t.Fatal(r.Fail)
	}
	m := read(t, w, active)
	if m.Phase != lifecycle.Done || m.Env != lifecycle.EnvDestroyed || m.Verdict != lifecycle.VerdictAborted {
		t.Fatalf("%+v", m)
	}
	if !slices.Contains(w.snapshot(), active+" down") || len(w.store.ReadState().State.Leases) != 0 {
		t.Fatalf("%v %v", w.snapshot(), w.store.ReadState().State.Leases)
	}
}

func TestDestroyingAKeptRunTakesItDownReleasesTheLeaseAndKeepsTheReport(t *testing.T) {
	s, w := setup(t, []store.Meta{preserved(1, 10)})
	dir, _ := w.store.RunDir(rid(1))
	for _, name := range []string{"src", "work", "env", "build", "report"} {
		os.MkdirAll(filepath.Join(dir, name), 0o700)
	}
	if r := s.Destroy(rid(1), Actor{By: "cli"}); !r.OK() {
		t.Fatal(r.Fail)
	}
	m := read(t, w, rid(1))
	if m.Env != lifecycle.EnvDestroyed || m.Verdict != lifecycle.VerdictPassed || m.DestroyReason != lifecycle.DestroyPerson {
		t.Fatalf("%+v", m)
	}
	if !reflect.DeepEqual(w.store.ReadState().State.Leases, store.Leases{}) {
		t.Fatalf("%v", w.store.ReadState().State.Leases)
	}
	if _, err := os.Stat(filepath.Join(dir, "report")); err != nil {
		t.Fatal("report removed")
	}
}

func TestAbortSignalsOnlyTheActiveRunAndAnyOtherRunIsAConflict(t *testing.T) {
	s, w := setup(t, []store.Meta{preserved(2, 10)})
	active := s.Submit(ctx, sub("c1")).ID
	s.Tick()
	if r := s.Abort(rid(2), Actor{By: "cli"}); r.Fail == nil || r.Fail.Status != 409 || r.Fail.Code != ticker.CodeConflictState {
		t.Fatalf("%+v", r)
	}
	if r := s.Abort(active, Actor{By: "cli"}); !r.OK() {
		t.Fatal(r.Fail)
	}
	s.Idle()
	if m := read(t, w, active); m.Phase != lifecycle.Done || m.Env != lifecycle.EnvStopped || m.Verdict != lifecycle.VerdictAborted || m.StopReason != lifecycle.StopDone {
		t.Fatalf("%+v", m)
	}
}

func TestRequestThatCollidesWithAnOperationInProgressIsAConflictWithTheCurrentState(t *testing.T) {
	s, _ := setup(t, []store.Meta{preserved(1, 10, func(m *store.Meta) { m.Phase = lifecycle.Stopping })})
	r := s.Destroy(rid(1), Actor{By: "cli"})
	if r.Fail == nil || r.Fail.Status != 409 || r.Fail.Code != ticker.CodeConflictState || r.Fail.Run.Phase != lifecycle.Stopping {
		t.Fatalf("%+v", r)
	}
}

func TestExecDenialStopsBeforeTheRunnerAndAnAllowedExecRunsUnderTheRunLock(t *testing.T) {
	s, w := setup(t, []store.Meta{preserved(1, 10), preserved(2, 11, func(m *store.Meta) { m.Env = lifecycle.EnvStopped })})
	if r := s.Exec(rid(2), "health", Actor{By: "cli"}); r.Fail == nil || !strings.Contains(r.Fail.Reason, "resume first") {
		t.Fatalf("%+v", r)
	}
	if len(suffixed(w.snapshot(), "runRunner health")) != 0 {
		t.Fatal("runner called for a denied exec")
	}
	if r := s.Exec(rid(1), "health", Actor{By: "cli"}); r.Fail != nil || r.Result != "health" {
		t.Fatalf("%+v", r)
	}
	if got := suffixed(w.snapshot(), "runRunner health"); !slices.Equal(got, []string{rid(1) + " runRunner health"}) {
		t.Fatalf("%v", got)
	}
}

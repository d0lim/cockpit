package store

import (
	"errors"
	"reflect"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

const runC = "run-20261012-153000-997a37e-3"

func TestApplyEventWritesTheNextStateWithExtrasAndAConflictWritesNothing(t *testing.T) {
	s := Open(homeFixture(t))
	mustWriteMeta(t, s, fixture(runA))
	got, err := s.ApplyEvent(runA, lifecycle.TickStart, lifecycle.Detail{}, func(m *Meta) { m.Slot = slot(4) })
	if err != nil || got.Phase != lifecycle.Preparing || *got.Slot != 4 {
		t.Fatalf("%+v %v", got, err)
	}
	before, _ := s.MustRead(runA)
	var conflict *lifecycle.Conflict
	current, err := s.ApplyEvent(runA, lifecycle.Resume, lifecycle.Detail{}, func(m *Meta) { m.Notice = "changed" })
	if !errors.As(err, &conflict) || current.Phase != lifecycle.Preparing {
		t.Fatalf("%+v %v", current, err)
	}
	after, _ := s.MustRead(runA)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("conflict changed the meta: %+v", after)
	}
}

func TestBrokenStateIsRebuiltWithTheQueueInSubmitOrderAndLeasesFromMetaSlots(t *testing.T) {
	runs := []Meta{
		fixture(runA, func(m *Meta) {
			m.State = lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictPassed}
			m.Slot = slot(1)
		}),
		fixture(runC, func(m *Meta) { m.CreatedAt = "2026-10-12T06:30:30.000Z" }),
		fixture(runB, func(m *Meta) { m.CreatedAt = "2026-10-12T06:30:20.000Z" }),
	}
	got := NormalizedState(runs, EmptyState(), "state.json is not JSON")
	want := HubState{SchemaVersion: 2, Queue: []string{runB, runC}, Leases: Leases{runA: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
}

func TestNormalizedStateKeepsQueueOrderDropsRunsThatLeftAndAppendsMissingOnes(t *testing.T) {
	runs := []Meta{
		fixture(runA, func(m *Meta) { m.CreatedAt = "2026-10-12T06:30:10.000Z" }),
		fixture(runB, func(m *Meta) {
			m.State = lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvAbsent, Verdict: lifecycle.VerdictError}
			m.Slot = slot(2)
		}),
		fixture(runC, func(m *Meta) { m.CreatedAt = "2026-10-12T06:30:00.000Z" }),
	}
	in := HubState{SchemaVersion: 2, Queue: []string{runA, runB, runA}, Leases: Leases{runB: 2}}
	got := NormalizedState(runs, in, "")
	want := HubState{SchemaVersion: 2, Queue: []string{runA, runC}, Leases: Leases{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
}

func TestHoldsLeaseIsTrueForActiveInProgressAndKeptRunsOnly(t *testing.T) {
	holds := []lifecycle.State{
		{Phase: lifecycle.Testing, Env: lifecycle.EnvUp},
		{Phase: lifecycle.Destroying, Env: lifecycle.EnvUp},
		{Phase: lifecycle.Done, Env: lifecycle.EnvUp},
		{Phase: lifecycle.Done, Env: lifecycle.EnvStopped},
	}
	free := []lifecycle.State{
		{Phase: lifecycle.Queued, Env: lifecycle.EnvAbsent},
		{Phase: lifecycle.Done, Env: lifecycle.EnvAbsent},
		{Phase: lifecycle.Done, Env: lifecycle.EnvDestroyed},
		{Phase: lifecycle.Done, Env: lifecycle.EnvAbsent, Mark: lifecycle.Lost},
	}
	for _, s := range holds {
		if !HoldsLease(fixture(runA, func(m *Meta) { m.State = s; m.Slot = slot(0) })) {
			t.Errorf("%+v does not hold", s)
		}
	}
	for _, s := range free {
		if HoldsLease(fixture(runA, func(m *Meta) { m.State = s; m.Slot = slot(0) })) {
			t.Errorf("%+v holds", s)
		}
	}
}

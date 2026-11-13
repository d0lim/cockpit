package store

import (
	"reflect"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

func TestStageTimesSurviveAHubRestart(t *testing.T) {
	home := homeFixture(t)
	s := Open(home)
	mustWriteMeta(t, s, fixture(runA, func(m *Meta) { m.State = lifecycle.State{Phase: lifecycle.Building, Env: lifecycle.EnvAbsent} }))
	if _, err := s.PatchMeta(runA, func(m *Meta) {
		m.StartStage(lifecycle.Preparing, "2026-10-12T06:30:01.000Z")
		m.EndStage("2026-10-12T06:30:04.000Z", contract.SpanStatusOK)
		m.StartStage(lifecycle.Building, "2026-10-12T06:30:05.000Z")
	}); err != nil {
		t.Fatal(err)
	}
	want := []StageTime{
		{Stage: lifecycle.Preparing, Start: "2026-10-12T06:30:01.000Z", End: "2026-10-12T06:30:04.000Z", Outcome: contract.SpanStatusOK},
		{Stage: lifecycle.Building, Start: "2026-10-12T06:30:05.000Z"},
	}
	m, err := Open(home).MustRead(runA)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.Stages, want) {
		t.Fatalf("stages after reopening the home %+v, want %+v", m.Stages, want)
	}
	if got := m.StageStart(); got != "2026-10-12T06:30:05.000Z" {
		t.Fatalf("the open build stage starts at %q", got)
	}
}

func TestEndingAStageTouchesOnlyTheOpenEntryAndAnEndedStageHasNoCurrentStart(t *testing.T) {
	m := fixture(runA, func(m *Meta) { m.State = lifecycle.State{Phase: lifecycle.Building, Env: lifecycle.EnvAbsent} })
	m.StartStage(lifecycle.Building, "2026-10-12T06:30:05.000Z")
	m.EndStage("2026-10-12T06:30:09.000Z", contract.SpanStatusError)
	m.EndStage("2026-10-12T06:31:00.000Z", contract.SpanStatusOK)
	if got := m.Stages[0]; got.End != "2026-10-12T06:30:09.000Z" || got.Outcome != contract.SpanStatusError {
		t.Fatalf("a second end rewrote the ended entry: %+v", got)
	}
	if got := m.StageStart(); got != "" {
		t.Fatalf("an ended stage still reports the start %q", got)
	}
	m.Phase = lifecycle.Sealing
	m.StartStage(lifecycle.Building, "2026-10-12T06:31:01.000Z")
	if got := m.StageStart(); got != "" {
		t.Fatalf("an open entry of another phase reports the start %q", got)
	}
}

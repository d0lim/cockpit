package reconcile

import (
	"reflect"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
)

func interruptedInSuite(m *store.Meta) {
	m.State = lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}
	m.StartedAt = at(10)
	m.Stages = []store.StageTime{
		{Stage: lifecycle.Booting, Start: "2026-10-12T06:10:00.000Z", End: "2026-10-12T06:11:00.000Z", Outcome: contract.SpanStatusOK},
		{Stage: lifecycle.Testing, Start: "2026-10-12T06:11:00.000Z"},
	}
}

func TestAnInterruptedRunEndsItsOpenStageWithAnError(t *testing.T) {
	w := setup(t, meta(rid(1), withSlot(1), interruptedInSuite))
	w.env.results[rid(1)] = true
	w.run(t)
	if len(w.env.reports) != 1 {
		t.Fatalf("reports %d", len(w.env.reports))
	}
	reported := w.env.reports[0].Stages
	if len(reported) != 2 {
		t.Fatalf("the report saw the stages %+v", reported)
	}
	suite := reported[1]
	if suite.Stage != lifecycle.Testing || suite.Start != "2026-10-12T06:11:00.000Z" || suite.End == "" || suite.Outcome != contract.SpanStatusError {
		t.Fatalf("the interrupted suite stage reached the report as %+v", suite)
	}
	if boot := reported[0]; boot.End != "2026-10-12T06:11:00.000Z" || boot.Outcome != contract.SpanStatusOK {
		t.Fatalf("canary: the ended boot stage changed to %+v", boot)
	}
	if m := w.read(t, rid(1)); !reflect.DeepEqual(m.Stages, reported) || m.Phase != lifecycle.Done {
		t.Fatalf("the record holds %+v in %+v", m.Stages, m.State)
	}
}

func TestAnInterruptedRunWithoutAReportStillEndsItsOpenStageWithAnError(t *testing.T) {
	w := setup(t, meta(rid(1), withSlot(1), interruptedInSuite))
	w.run(t)
	m := w.read(t, rid(1))
	if len(w.env.reports) != 0 || m.Phase != lifecycle.Done || m.Verdict != lifecycle.VerdictError {
		t.Fatalf("reports %d, state %+v", len(w.env.reports), m.State)
	}
	if suite := m.Stages[1]; suite.End == "" || suite.Outcome != contract.SpanStatusError {
		t.Fatalf("the interrupted suite stage is %+v", suite)
	}
}

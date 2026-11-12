package pipeline

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/signal"
)

func streamSpans(t *testing.T, h *Hub, id string) map[string]contract.Signal {
	t.Helper()
	resp, err := h.Streams().Read(id, signal.ReadRequest{Limit: 5000})
	if err != nil {
		t.Fatal(err)
	}
	return spansOf(resp.Signals, id)
}

func stagePhases(m store.Meta) []lifecycle.Phase {
	var out []lifecycle.Phase
	for _, st := range m.Stages {
		out = append(out, st.Stage)
	}
	return out
}

func assertStageMatchesRecord(t *testing.T, spans map[string]contract.Signal, st store.StageTime) {
	t.Helper()
	sig, ok := spans[signal.StageSpanPrefix+stageNames[st.Stage]]
	if !ok || sig.Span.Start != st.Start || sig.Span.End != st.End || sig.Span.Status != st.Outcome || sig.TSSource != contract.TimeSourceHost {
		t.Fatalf("stage %s: span %+v, record %+v", st.Stage, sig.Span, st)
	}
}

func TestEveryStageTheLoopRanBecomesASpanUnderThePipelineRoot(t *testing.T) {
	w, url := spanWorld(t, false)
	m := w.submit()
	if m.Phase != lifecycle.Done || m.FailedStage != "" {
		t.Fatalf("state %+v failed stage %q", m.State, m.FailedStage)
	}
	want := []lifecycle.Phase{lifecycle.Preparing, lifecycle.Building, lifecycle.Sealing, lifecycle.Booting, lifecycle.Testing, lifecycle.Reporting}
	if got := stagePhases(m); !slices.Equal(got, want) {
		t.Fatalf("recorded stages %v, want %v", got, want)
	}
	live := spansOf(readSignals(t, url, m.ID).Signals, m.ID)
	frozen := spansOf(frozenSignals(t, w.hub, m.ID), m.ID)
	assertLiveTree(t, "API read", live, pipelineTree(liveStages))
	assertTree(t, "frozen copy", frozen, pipelineTree(frozenStages))
	for i, st := range m.Stages {
		if st.End == "" || st.Outcome != contract.SpanStatusOK || st.Start > st.End || (i > 0 && st.Start < m.Stages[i-1].End) {
			t.Fatalf("stage %d %+v after %+v", i, st, m.Stages[max(0, i-1)])
		}
		assertStageMatchesRecord(t, live, st)
		if st.Stage != lifecycle.Reporting {
			assertStageMatchesRecord(t, frozen, st)
		}
	}
	report := m.Stages[len(m.Stages)-1]
	if root := live[signal.PipelineSpanID].Span; root.Start != m.Stages[0].Start || root.End != report.End || root.Status != contract.SpanStatusOK {
		t.Fatalf("live pipeline root %+v, want it to end with the report at %s", root, report.End)
	}
	if root := frozen[signal.PipelineSpanID].Span; root.End != m.Stages[4].End || root.End >= report.End {
		t.Fatalf("frozen pipeline root %+v, want it to end with the suite at %s", root, m.Stages[4].End)
	}
	if _, has := frozen[signal.StageSpanPrefix+"report"]; has {
		t.Fatal("the frozen copy holds the report stage, which was still open at the freeze")
	}
}

func TestAFailedStageEndsWithAnErrorSpanAndLaterStagesHaveNone(t *testing.T) {
	w := newWorld(t, func(f *fakedocker.Fake) { f.FailVerb = map[string]bool{"compose build": true} }, nil)
	m := w.submit()
	if m.Phase != lifecycle.Done || m.FailedStage != lifecycle.Building {
		t.Fatalf("state %+v failed stage %s", m.State, m.FailedStage)
	}
	if got := stagePhases(m); !slices.Equal(got, []lifecycle.Phase{lifecycle.Preparing, lifecycle.Building}) {
		t.Fatalf("recorded stages %v", got)
	}
	spans := streamSpans(t, w.hub, m.ID)
	assertTree(t, "stream", spans, map[string]string{signal.PipelineSpanID: "", "stage:prepare": signal.PipelineSpanID, "stage:build": signal.PipelineSpanID})
	for _, st := range m.Stages {
		assertStageMatchesRecord(t, spans, st)
	}
	if build := spans["stage:build"].Span; build.Status != contract.SpanStatusError {
		t.Fatalf("the failed build stage is %s", build.Status)
	}
	if prepare := spans["stage:prepare"].Span; prepare.Status != contract.SpanStatusOK {
		t.Fatalf("canary: the prepare stage is %s", prepare.Status)
	}
	if root := spans[signal.PipelineSpanID].Span; root.Status != contract.SpanStatusError || root.End != m.Stages[1].End {
		t.Fatalf("pipeline root %+v", root)
	}
}

func TestAnAbortedStageEndsAsIncomplete(t *testing.T) {
	w := newWorldWith(t, func(o *Options) { o.Limits.Boot = time.Minute }, func(f *fakedocker.Fake) { f.Unhealthy = true }, nil)
	id, done := w.startInBackground()
	waitFor(t, "the boot wait", func() bool { return indexOf(w.verbs(), "compose ps") >= 0 && w.meta(id).Phase == lifecycle.Booting })
	if got := w.meta(id).StageStart(); got == "" {
		t.Fatal("the boot stage has no open start while it runs")
	}
	if r := w.sched.Abort(id, scheduler.Actor{By: "tester"}); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	finish(t, done)
	m := w.meta(id)
	boot := m.Stages[len(m.Stages)-1]
	if boot.Stage != lifecycle.Booting || boot.Outcome != contract.SpanStatusIncomplete || boot.End == "" {
		t.Fatalf("the aborted boot stage %+v", boot)
	}
	if spans := streamSpans(t, w.hub, id); spans["stage:boot"].Span == nil || spans["stage:boot"].Span.Status != contract.SpanStatusIncomplete || spans["stage:seal"].Span.Status != contract.SpanStatusOK {
		t.Fatalf("stage spans %v", slices.Sorted(maps.Keys(spans)))
	}
}

func TestTheStageNamesCoverExactlyThePhasesThePipelineRuns(t *testing.T) {
	labels := lifecycle.StageLabels()
	if len(labels) != 6 || len(stageNames) != len(labels) {
		t.Fatalf("%d stage labels, %d stage names", len(labels), len(stageNames))
	}
	for p := range labels {
		if stageFor(p) == nil || stageNames[p] == "" {
			t.Errorf("phase %s: stage %v, name %q", p, stageFor(p) != nil, stageNames[p])
		}
	}
	for p := range stageNames {
		if _, ok := labels[p]; !ok {
			t.Errorf("stage name for %s, which is no pipeline stage", p)
		}
	}
	for _, p := range []lifecycle.Phase{lifecycle.Queued, lifecycle.Done, lifecycle.Stopping} {
		if stageFor(p) != nil || stageNames[p] != "" {
			t.Errorf("canary: %s runs as a stage", p)
		}
	}
	if signal.StageSpanPrefix+stageNames[lifecycle.Testing] != signal.SuiteStageSpanID {
		t.Fatalf("the testing stage span is not the parent of the run span %s", signal.SuiteStageSpanID)
	}
}

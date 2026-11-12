package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/signal"
	"github.com/d0lim/cockpit/internal/tracectx"
)

var otlpPipelineMutations = []porttest.Mutation{
	{"the suite waits for the OTLP stream to go quiet", "drop the drainOTLP call in runSuite", "red: the spans and the metric total written after the runner exit are missing from the frozen copy", "TestTheSuiteWaitsForTheOTLPStreamToGoQuiet"},
	{"service spans move under their call before the freeze", "drop the Reparent call in reportSpans", "red: the frozen service spans keep their w3c parents", "TestServiceSpansAreReparentedUnderTheirCallBeforeTheFreeze"},
	{"a run without the OTLP observer waits no drain", "make DrainOTLP sleep the quiet window when the run has no observer", "red: a run with NoCollection writes an otlp drain line", "TestARunWithoutCollectionHasNoOTLPDrainAndNoOTLPSignals"},
}

func TestOTLPPipelineMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "otlp_test.go", otlpPipelineMutations)
}

const (
	createSpanID = "00f067aa0ba902b7"
	readSpanID   = "1a2b3c4d5e6f7081"
)

func tracedSteps() []contract.ResultStep {
	created, read := 201, 200
	steps := passingSteps()
	steps[2].StartedAt, steps[2].FinishedAt = "2026-10-19T01:00:02.000Z", "2026-10-19T01:00:03.000Z"
	steps[2].Calls = []contract.StepCall{
		{Method: "POST", Host: "app", Path: "/orders", Status: &created, StartedAt: "2026-10-19T01:00:02.100Z", DurationMs: 30, SpanID: createSpanID},
		{Method: "GET", Host: "app", Path: "/orders/1", Status: &read, StartedAt: "2026-10-19T01:00:02.200Z", DurationMs: 10, SpanID: readSpanID},
	}
	return steps
}

func otlpLine(t *testing.T, sig map[string]any) string {
	data, err := json.Marshal(map[string]any{"v": 1, "signal": sig})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func appSpan(t *testing.T, runID, id, parent string) string {
	return otlpLine(t, map[string]any{"kind": "span", "id": id, "ts": "2026-10-19T01:00:02.110Z", "tsSource": "container", "attrs": map[string]any{"service": "app"},
		"span": map[string]any{"traceId": "w3c:" + tracectx.TraceID(runID), "parentId": parent, "name": "app " + id, "start": "2026-10-19T01:00:02.110Z", "end": "2026-10-19T01:00:02.120Z", "status": "ok"}})
}

func appTotal(t *testing.T, rev int64, value float64) string {
	return otlpLine(t, map[string]any{"kind": "metric", "id": "metric:00aa", "rev": rev, "ts": "2026-10-19T01:00:02Z", "tsSource": "container", "attrs": map[string]any{"service": "app"},
		"metric": map[string]any{"name": "app.requests", "value": value, "type": "sum"}})
}

func otlpLines(t *testing.T) func(runID string) []fakedocker.OTLPLine {
	return func(runID string) []fakedocker.OTLPLine {
		return []fakedocker.OTLPLine{
			{After: 0, Line: appSpan(t, runID, "w3c:00000000000000c1", "w3c:"+createSpanID)},
			{After: 0, Line: appSpan(t, runID, "w3c:00000000000000c2", "w3c:00000000000000c1")},
			{After: 0, Line: appTotal(t, 1000, 1)},
			{After: 200 * time.Millisecond, Line: appSpan(t, runID, "w3c:00000000000000d1", "w3c:"+readSpanID)},
			{After: 250 * time.Millisecond, Line: appTotal(t, 2000, 2)},
			{After: 400 * time.Millisecond, Line: appSpan(t, runID, "w3c:00000000000000d2", "w3c:00000000000000d1")},
		}
	}
}

func otlpWorld(t *testing.T, collecting bool) *world {
	return newWorldWith(t, func(o *Options) {
		o.Limits.OTLPQuiet, o.Limits.OTLPDrain = 300*time.Millisecond, 3*time.Second
		if collecting {
			o.NoCollection = false
			o.Collect = collect.Options{DrainWait: 2 * time.Second, Heartbeat: 100 * time.Millisecond}
		}
	}, func(f *fakedocker.Fake) {
		f.Steps, f.OTLP = tracedSteps(), otlpLines(t)
	}, func(dir string) {
		os.RemoveAll(filepath.Join(dir, "scripts", "collectors"))
		writeRepoFile(t, filepath.Join(dir, "scripts", "collectors", ".keep"), "")
	})
}

func otlpOf(signals []contract.Signal) (map[string]contract.Signal, []contract.Signal) {
	spans := map[string]contract.Signal{}
	var metrics []contract.Signal
	for _, sig := range signals {
		switch {
		case sig.Collector != signal.OTLPCollector:
		case sig.Span != nil:
			spans[sig.ID] = sig
		case sig.Metric != nil:
			metrics = append(metrics, sig)
		}
	}
	return spans, metrics
}

func reportedRun(t *testing.T, w *world) (string, []contract.Signal) {
	t.Helper()
	m := w.submit()
	if m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("verdict %s: %s\n%s", m.Verdict, m.Notice, w.eventText())
	}
	return m.ID, frozenSignals(t, w.hub, m.ID)
}

func TestTheSuiteWaitsForTheOTLPStreamToGoQuiet(t *testing.T) {
	w := otlpWorld(t, true)
	_, frozen := reportedRun(t, w)
	spans, metrics := otlpOf(frozen)
	if _, late := spans["w3c:00000000000000d2"]; !late || len(spans) != 4 {
		t.Fatalf("frozen otlp spans %d: %v", len(spans), spans)
	}
	if len(metrics) != 1 || metrics[0].Metric.Value != 2 || metrics[0].Rev != 2000 {
		t.Fatalf("frozen otlp metric winners %+v", metrics)
	}
	if text := w.eventText(); !strings.Contains(text, "otlp drained after") {
		t.Fatalf("no drain line in\n%s", text)
	}
}

func TestServiceSpansAreReparentedUnderTheirCallBeforeTheFreeze(t *testing.T) {
	w := otlpWorld(t, true)
	runID, frozen := reportedRun(t, w)
	spans, _ := otlpOf(frozen)
	want := map[string]string{
		"w3c:00000000000000c1": "call:orders/create/1",
		"w3c:00000000000000c2": "w3c:00000000000000c1",
		"w3c:00000000000000d1": "call:orders/create/2",
		"w3c:00000000000000d2": "w3c:00000000000000d1",
	}
	calls := spansOf(frozen, runID)
	for id, parent := range want {
		got, ok := spans[id]
		if !ok || got.Span.ParentID != parent || got.Span.TraceID != runID {
			t.Fatalf("%s: %+v", id, got.Span)
		}
		if strings.HasPrefix(parent, "call:") && calls[parent].Collector != signal.RunnerSource {
			t.Fatalf("the parent %s of %s is not a runner call span in the frozen copy: %v", parent, id, calls[parent])
		}
	}
	if len(want) != 4 || len(spans) != len(want) {
		t.Fatalf("checked %d of %d frozen otlp spans", len(want), len(spans))
	}
}

func TestARunWithoutCollectionHasNoOTLPDrainAndNoOTLPSignals(t *testing.T) {
	w := otlpWorld(t, false)
	_, frozen := reportedRun(t, w)
	spans, metrics := otlpOf(frozen)
	if len(spans) != 0 || len(metrics) != 0 || strings.Contains(w.eventText(), "otlp drain") {
		t.Fatalf("spans %v metrics %v events\n%s", spans, metrics, w.eventText())
	}
	if !strings.Contains(w.eventText(), "health › health-ok passed") {
		t.Fatalf("canary: the runner lines are missing from\n%s", w.eventText())
	}
}

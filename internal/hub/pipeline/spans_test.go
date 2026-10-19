package pipeline

import (
	"encoding/json"
	"maps"
	"net"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/signal"
)

func timedSteps() []contract.ResultStep {
	return []contract.ResultStep{
		{Flow: "health", ID: "health-ok", Status: contract.StepStatusPassed, StartedAt: "2026-10-19T01:00:01.000Z", FinishedAt: "2026-10-19T01:00:02.000Z"},
		{Flow: "orders", ID: "create", Status: contract.StepStatusPassed, StartedAt: "2026-10-19T01:00:02.000Z", FinishedAt: "2026-10-19T01:00:03.000Z"},
		{Flow: "orders", ID: "read", Status: contract.StepStatusRunning, StartedAt: "2026-10-19T01:00:03.000Z"},
	}
}

func steppingClock() func() time.Time {
	var mu sync.Mutex
	at := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		at = at.Add(time.Second)
		return at
	}
}

func spanWorld(t *testing.T, incomplete bool) (*world, string) {
	w := newWorldWith(t, func(o *Options) { o.Now = steppingClock() }, func(f *fakedocker.Fake) {
		f.Steps, f.Incomplete = timedSteps(), incomplete
		if !incomplete {
			f.Steps[2].Status, f.Steps[2].FinishedAt = contract.StepStatusPassed, "2026-10-19T01:00:04.000Z"
		}
	}, nil)
	srv := httptest.NewUnstartedServer(nil)
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	srv.Config.Handler = api.Handler(api.Deps{Port: port, Store: w.hub.Store(), Scheduler: w.sched, Signals: w.hub.Streams(), Collectors: w.hub.Collectors(), Mask: w.hub.Mask})
	srv.Start()
	t.Cleanup(srv.Close)
	return w, srv.URL
}

func spansOf(signals []contract.Signal, trace string) map[string]contract.Signal {
	out := map[string]contract.Signal{}
	for _, sig := range signals {
		if sig.Kind == contract.SignalKindSpan && sig.Span.TraceID == trace {
			out[sig.ID] = sig
		}
	}
	return out
}

func frozenSignals(t *testing.T, h *Hub, id string) []contract.Signal {
	t.Helper()
	dir, _ := h.Store().RunDir(id)
	var out []contract.Signal
	for _, line := range strings.Split(strings.TrimSpace(fileText(t, filepath.Join(dir, report.DirReport, "signals", "stream.jsonl"))), "\n") {
		if line == "" {
			continue
		}
		var sig contract.Signal
		if err := json.Unmarshal([]byte(line), &sig); err != nil {
			t.Fatalf("frozen line %q: %v", line, err)
		}
		out = append(out, sig)
	}
	return out
}

var runTree = map[string]string{"run": "", "flow:health": "run", "flow:orders": "run", "step:health/health-ok": "flow:health", "step:orders/create": "flow:orders", "step:orders/read": "flow:orders"}

func assertTree(t *testing.T, where string, spans map[string]contract.Signal, tree map[string]string) {
	t.Helper()
	if len(spans) != len(tree) {
		t.Fatalf("%s: %d spans, want %d: %v", where, len(spans), len(tree), slices.Sorted(maps.Keys(spans)))
	}
	for id, parent := range tree {
		sig, ok := spans[id]
		if !ok || sig.Span.ParentID != parent || sig.Collector != signal.RunnerSource {
			t.Fatalf("%s: span %s = %+v, want parent %q", where, id, sig.Span, parent)
		}
		if parent != "" && spans[parent].Seq >= sig.Seq {
			t.Errorf("%s: parent %s (seq %d) is not before %s (seq %d)", where, parent, spans[parent].Seq, id, sig.Seq)
		}
	}
}

func TestARunnerResultBecomesRunFlowAndStepSpansInTheAPIReadAndTheFrozenCopyParentsFirst(t *testing.T) {
	w, url := spanWorld(t, false)
	m := w.submit()
	live := spansOf(readSignals(t, url, m.ID).Signals, m.ID)
	assertTree(t, "API read", live, runTree)
	assertTree(t, "frozen copy", spansOf(frozenSignals(t, w.hub, m.ID), m.ID), runTree)
	for id, sig := range live {
		if sig.Span.Status != contract.SpanStatusOK {
			t.Errorf("span %s status %s, want ok", id, sig.Span.Status)
		}
	}
	if run := live["run"].Span; run.Start != "2026-10-19T01:00:00.000Z" || run.Name != "smoke" {
		t.Fatalf("run span %+v", run)
	}
}

func TestAnExecAddsItsOwnTraceLinkedToTheRunSpan(t *testing.T) {
	w, url := spanWorld(t, false)
	m := w.submit()
	if r := w.sched.Exec(m.ID, "health", scheduler.Actor{By: "tester"}); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	var traces []string
	for _, sig := range readSignals(t, url, m.ID).Signals {
		if sig.Kind == contract.SignalKindSpan && strings.HasPrefix(sig.Span.TraceID, m.ID+"/exec/") && !slices.Contains(traces, sig.Span.TraceID) {
			traces = append(traces, sig.Span.TraceID)
		}
	}
	if len(traces) != 1 || !strings.HasSuffix(traces[0], "-health") {
		t.Fatalf("exec traces %v", traces)
	}
	view := readSignals(t, url, m.ID).Signals
	exec := spansOf(view, traces[0])
	tree := map[string]string{"exec": ""}
	for id, parent := range runTree {
		if id != "run" {
			tree[id] = parent
			if parent == "run" {
				tree[id] = "exec"
			}
		}
	}
	assertTree(t, "exec trace", exec, tree)
	if links := exec["exec"].Span.Links; len(links) != 1 || links[0] != (contract.SpanLink{TraceID: m.ID, SpanID: signal.RunSpanID}) {
		t.Fatalf("exec root links %+v", links)
	}
	if len(spansOf(view, m.ID)) != len(runTree) {
		t.Fatalf("the exec changed the run trace: %d spans", len(spansOf(view, m.ID)))
	}
}

func TestARunnerKilledMidStepYieldsIncompleteSpansThatEndAtTheContainerExit(t *testing.T) {
	w, url := spanWorld(t, true)
	m := w.submit()
	spans := spansOf(readSignals(t, url, m.ID).Signals, m.ID)
	assertTree(t, "API read", spans, runTree)
	exit := spans["step:orders/read"].Span.End
	dir, _ := w.hub.Store().RunDir(m.ID)
	doc, _ := readSummary(t, dir)
	if generated, _ := doc["generatedAt"].(string); generated == "" || exit <= "2026-10-19T01:00:03.000Z" || exit >= generated {
		t.Fatalf("the running step ends at %q, not between its start and the report at %q", exit, generated)
	}
	want := map[string]contract.SpanStatus{
		"run": contract.SpanStatusIncomplete, "flow:orders": contract.SpanStatusIncomplete, "step:orders/read": contract.SpanStatusIncomplete,
		"flow:health": contract.SpanStatusOK, "step:health/health-ok": contract.SpanStatusOK, "step:orders/create": contract.SpanStatusOK,
	}
	for id, status := range want {
		if got := spans[id].Span; got.Status != status {
			t.Errorf("span %s status %s, want %s", id, got.Status, status)
		}
	}
	for _, id := range []string{"run", "flow:orders"} {
		if end := spans[id].Span.End; end != exit {
			t.Errorf("span %s ends at %q, want the container exit %q", id, end, exit)
		}
	}
	if end := spans["step:orders/create"].Span.End; end != "2026-10-19T01:00:03.000Z" {
		t.Errorf("a finished step moved its end to %q", end)
	}
}

package observer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
	dockerport "github.com/d0lim/cockpit/internal/ports/docker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/signal"
	"github.com/d0lim/cockpit/internal/tracectx"
)

var otlpMutations = []porttest.Mutation{
	{"OTLP signals reach the stream only through the masking Append", "make signal.Stream.Append store json.Marshal(sig) in place of maskSignal", "red: the stream file holds the planted value", "TestAPlantedCredentialInAServiceSpanAttributeIsMaskedWhileAPlainAttributeReadsBack"},
	{"the run's W3C trace id maps to the run trace", "drop the registerTrace call in OTLP.defaults", "red: the span keeps the w3c trace id of the run", "TestAnOTLPLineBecomesASpanInTheRunTrace"},
	{"the observer follows the container of the otlp role", "ask RoleContainers for the edge role in OTLP.Run", "red: c-otlp is never followed and no span arrives", "TestAnOTLPLineBecomesASpanInTheRunTrace"},
	{"a metric gets the table of its name", "drop the table attribute in OTLP.parse", "red: the metric has no table attribute", "TestAnOTLPLineBecomesASpanInTheRunTrace"},
	{"unknown trace ids keep their w3c form", "return the run id from traceFor for every unknown id", "red: the foreign span joins the run trace", "TestAnUnknownTraceIDKeepsItsOwnTraceAndARegisteredExecMapsToItsTrace"},
	{"broken lines are counted by reason", "drop the count calls in OTLP.line", "red: the state holds no unparsed or refused counts", "TestABrokenLineIsCountedAndTheFollowContinues"},
	{"a line over 64 KiB is not stored", "drop the length check in OTLP.parse", "red: the oversized span is stored", "TestABrokenLineIsCountedAndTheFollowContinues"},
	{"a follow that ends while the receiver ran is recorded as a gap", "drop the gap call in OTLP.Run", "red: no gap event names the cursor", "TestAFollowThatEndsWhileTheReceiverRanRecordsAGapAndAnExitedReceiverIsReadOnce"},
	{"the cursor survives a restart", "skip load in OTLP.Run", "red: the second observer follows without --since and stores the delta points twice", "TestARestartedObserverResumesFromItsSavedCursor"},
	{"a resumed follow skips the lines at or before its floor", "skip the floor check in OTLP.line", "red: the delta point at the cursor is stored twice", "TestARestartedObserverResumesFromItsSavedCursor"},
	{"a line cut off by a stopped follow is read whole by the next one", "flush the line writer at the end of OTLP.follow", "red: the cursor moves to the cut line and the next follow skips it", "TestACancelledFollowLeavesAPartLineForTheNextFollow"},
	{"the settle waits a quiet window counted from its own start", "start the quiet window at the last span heard even before the wait", "red: the settle after an earlier span returns before the quiet window", "TestSettleWaitsAQuietWindowFromItsStartAndStopsAtTheLimitWhileSpansKeepComing"},
	{"only spans of the run's traces extend the settle", "store the heard time for every parsed line in OTLP.line", "red: the settle runs to its limit while metric lines keep coming", "TestSettleWaitsAQuietWindowFromItsStartAndStopsAtTheLimitWhileSpansKeepComing"},
	{"spans of a foreign trace do not extend the settle", "store the heard time for every parsed span in OTLP.line", "red: the settle runs to its limit while spans of a foreign trace keep coming", "TestSettleWaitsAQuietWindowFromItsStartAndStopsAtTheLimitWhileSpansKeepComing"},
}

func TestOTLPMutationRecordMatchesTestsInTheseFiles(t *testing.T) {
	porttest.AssertMutationRecordAcross(t, []string{"otlp_test.go", "otlpresume_test.go"}, otlpMutations)
}

func storeIn(t *testing.T, dir string) *signal.Stream {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	s, err := signal.Open(dir, signal.Options{Masker: func() *mask.Masker { return m }, Caps: signal.DefaultCaps()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func otlpPS(state string) string {
	return strings.Join([]string{"c-otlp", project + "-otlp-1", state, "Up", project, "otlp", runID, compose.RoleOTLP}, "\t")
}

func otlpOut(t *testing.T, sig map[string]any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"v": 1, "signal": sig})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func serviceSpan(t *testing.T, trace, id, parent string, attrs map[string]any) string {
	span := map[string]any{"traceId": trace, "name": "GET /orders", "start": "2026-10-19T01:00:01Z", "end": "2026-10-19T01:00:02Z", "status": "ok"}
	if parent != "" {
		span["parentId"] = parent
	}
	return otlpOut(t, map[string]any{"kind": "span", "id": id, "ts": "2026-10-19T01:00:01Z", "tsSource": "container", "attrs": attrs, "span": span})
}

func deltaPoint(t *testing.T, value int) string {
	return otlpOut(t, map[string]any{"kind": "metric", "ts": "2026-10-19T01:00:01Z", "tsSource": "container", "attrs": map[string]any{"service": "app"},
		"metric": map[string]any{"name": "app.requests", "value": value, "type": "sum"}})
}

type otlpWorld struct {
	d      *fakeDocker
	s      *signal.Stream
	obs    *OTLP
	cancel context.CancelFunc
	done   chan error
}

func newOTLPWorld(t *testing.T, ps []string, lines func(call int, since string) ([]string, bool)) *otlpWorld {
	t.Helper()
	d := &fakeDocker{ps: ps}
	var mu sync.Mutex
	calls := 0
	d.handle = func(ctx context.Context, cmd compose.Command) (compose.Result, error) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		since := ""
		for _, a := range cmd.Args {
			if v, ok := strings.CutPrefix(a, "--since="); ok {
				since = v
			}
		}
		out, block := lines(n, since)
		for i, l := range out {
			fmt.Fprintf(cmd.Stdout, "%s %s\n", stamp(i+1), l)
		}
		if block {
			<-ctx.Done()
		}
		return compose.Result{}, nil
	}
	return &otlpWorld{d: d, s: store(t, signal.DefaultCaps())}
}

func (w *otlpWorld) start(t *testing.T) *OTLP {
	t.Helper()
	dp, err := dockerport.New(dockerport.Options{Project: project, RunID: runID, Docker: w.d})
	if err != nil {
		t.Fatal(err)
	}
	w.obs = &OTLP{RunID: runID, Lister: dp, Docker: w.d, Stream: w.s, Poll: 20 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel, w.done = cancel, make(chan error, 1)
	go func() { w.done <- w.obs.Run(ctx) }()
	return w.obs
}

func (w *otlpWorld) stop(t *testing.T) {
	t.Helper()
	w.cancel()
	if err := <-w.done; err != nil {
		t.Fatal(err)
	}
}

func (w *otlpWorld) waitSignals(t *testing.T, n int) []contract.Signal {
	t.Helper()
	waitFor(t, fmt.Sprintf("%d otlp signals", n), func() bool { return len(events(t, w.s)) >= n })
	return events(t, w.s)
}

func byID(signals []contract.Signal) map[string]contract.Signal {
	out := map[string]contract.Signal{}
	for _, sig := range signals {
		out[sig.ID] = sig
	}
	return out
}

func TestAnOTLPLineBecomesASpanInTheRunTrace(t *testing.T) {
	trace := "w3c:" + tracectx.TraceID(runID)
	metric := otlpOut(t, map[string]any{"kind": "metric", "id": "metric:00ff", "rev": 1760835601000, "ts": "2026-10-19T01:00:01Z", "tsSource": "container",
		"attrs": map[string]any{"service": "app"}, "metric": map[string]any{"name": "app.requests", "value": 3, "type": "sum"}})
	w := newOTLPWorld(t, []string{psLine("c-app", "app", project, runID, ""), otlpPS("running")}, func(int, string) ([]string, bool) {
		return []string{serviceSpan(t, trace, "w3c:00000000000000aa", "w3c:00f067aa0ba902b7", map[string]any{"service": "app", "http.route": "/orders"}), metric}, true
	})
	w.start(t)
	got := byID(w.waitSignals(t, 2))
	w.stop(t)
	span := got["w3c:00000000000000aa"]
	if span.Collector != signal.OTLPCollector || span.Span == nil || span.Span.TraceID != runID || span.Span.ParentID != "w3c:00f067aa0ba902b7" || span.Attrs["service"] != "app" || span.TSSource != contract.TimeSourceContainer {
		t.Fatalf("span %+v %+v", span, span.Span)
	}
	if m := got["metric:00ff"]; m.Collector != signal.OTLPCollector || m.Metric == nil || m.Metric.Value != 3 || m.Rev != 1760835601000 || m.Attrs["table"] != "app.requests" || m.Attrs["service"] != "app" {
		t.Fatalf("canary: metric %+v", m)
	}
	followed := w.d.logsFor()
	if followed["c-otlp"] == 0 || followed["c-app"] != 0 {
		t.Fatalf("followed %v", followed)
	}
}

func TestAnUnknownTraceIDKeepsItsOwnTraceAndARegisteredExecMapsToItsTrace(t *testing.T) {
	foreign := "w3c:" + strings.Repeat("ab", 16)
	exec := "w3c:" + tracectx.ExecTraceID(runID, "20261019-010000-orders")
	w := newOTLPWorld(t, []string{otlpPS("running")}, func(int, string) ([]string, bool) {
		return []string{serviceSpan(t, foreign, "w3c:0000000000000001", "", map[string]any{"service": "app"}), serviceSpan(t, exec, "w3c:0000000000000002", "", map[string]any{"service": "app"})}, true
	})
	obs := w.start(t)
	obs.TraceExec("20261019-010000-orders")
	got := byID(w.waitSignals(t, 2))
	w.stop(t)
	if span := got["w3c:0000000000000001"]; span.Span.TraceID != foreign {
		t.Fatalf("the foreign span is in trace %q", span.Span.TraceID)
	}
	if span := got["w3c:0000000000000002"]; span.Span.TraceID != runID+"/exec/20261019-010000-orders" {
		t.Fatalf("canary: the exec span is in trace %q", span.Span.TraceID)
	}
}

func TestABrokenLineIsCountedAndTheFollowContinues(t *testing.T) {
	trace := "w3c:" + tracectx.TraceID(runID)
	badStatus := strings.Replace(serviceSpan(t, trace, "w3c:00000000000000bb", "", map[string]any{"service": "app"}), `"status":"ok"`, `"status":"fine"`, 1)
	broken := []string{"not json", `{"v":2,"signal":{"kind":"event"}}`, `{"v":1}`, serviceSpan(t, trace, "w3c:00000000000000ee", "", map[string]any{"service": "app", "pad": strings.Repeat("x", OTLPMaxLine)})}
	w := newOTLPWorld(t, []string{otlpPS("running")}, func(int, string) ([]string, bool) {
		return append(append(broken, badStatus), serviceSpan(t, trace, "w3c:00000000000000cc", "", map[string]any{"service": "app"})), true
	})
	w.start(t)
	got := byID(w.waitSignals(t, 1))
	w.stop(t)
	if _, ok := got["w3c:00000000000000cc"]; !ok || len(got) != 1 || len(broken) != 4 {
		t.Fatalf("stored %v after %d broken lines", got, len(broken))
	}
	state, err := w.s.KVGet(signal.OTLPCollector)
	if err != nil {
		t.Fatal(err)
	}
	dropped, _ := state["dropped"].(map[string]any)
	if dropped[DropUnparsed] != float64(4) || dropped[DropRefused] != float64(1) || state["cursor"] != stamp(6) {
		t.Fatalf("state %v", state)
	}
}

func TestAPlantedCredentialInAServiceSpanAttributeIsMaskedWhileAPlainAttributeReadsBack(t *testing.T) {
	trace := "w3c:" + tracectx.TraceID(runID)
	w := newOTLPWorld(t, []string{otlpPS("running")}, func(int, string) ([]string, bool) {
		return []string{serviceSpan(t, trace, "w3c:00000000000000dd", "", map[string]any{"service": "app", "db.statement": "select * where key='" + planted + "'", "http.route": "/orders"})}, true
	})
	dir := filepath.Join(t.TempDir(), "run")
	w.s = storeIn(t, dir)
	w.start(t)
	span := byID(w.waitSignals(t, 1))["w3c:00000000000000dd"]
	w.stop(t)
	raw, err := os.ReadFile(filepath.Join(dir, signal.Dir, "stream.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	stmt, _ := span.Attrs["db.statement"].(string)
	if strings.Contains(string(raw), planted) || !strings.Contains(string(raw), "****") || strings.Contains(stmt, planted) || !strings.Contains(stmt, "****") {
		t.Fatalf("stored %s, read db.statement %q", raw, stmt)
	}
	if span.Attrs["http.route"] != "/orders" || span.ID != "w3c:00000000000000dd" || span.Span.TraceID != runID {
		t.Fatalf("canary: %+v %+v", span.Attrs, span.Span)
	}
}

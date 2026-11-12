package observer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/signal"
	"github.com/d0lim/cockpit/internal/tracectx"
)

func TestAFollowThatEndsWhileTheReceiverRanRecordsAGapAndAnExitedReceiverIsReadOnce(t *testing.T) {
	trace := "w3c:" + tracectx.TraceID(runID)
	var w *otlpWorld
	w = newOTLPWorld(t, []string{otlpPS("running")}, func(call int, _ string) ([]string, bool) {
		if call == 1 {
			w.d.ps = []string{otlpPS("exited")}
			return []string{serviceSpan(t, trace, "w3c:0000000000000001", "", map[string]any{"service": "app"})}, false
		}
		return nil, false
	})
	w.start(t)
	waitFor(t, "the exited receiver read once", func() bool { return w.d.logsFor()["c-otlp"] >= 2 })
	time.Sleep(100 * time.Millisecond)
	w.stop(t)
	var gaps []contract.Signal
	for _, sig := range events(t, w.s) {
		if sig.Kind == contract.SignalKindEvent {
			gaps = append(gaps, sig)
		}
	}
	if len(gaps) != 1 || gaps[0].Collector != signal.OTLPCollector || gaps[0].Attrs["status"] != "gap" || gaps[0].Attrs["from"] != stamp(1) {
		t.Fatalf("gap events %+v", gaps)
	}
	if n := w.d.logsFor()["c-otlp"]; n != 2 {
		t.Fatalf("the receiver was followed %d times, want once running and once exited", n)
	}
}

func TestARestartedObserverResumesFromItsSavedCursor(t *testing.T) {
	first := newOTLPWorld(t, []string{otlpPS("running")}, func(int, string) ([]string, bool) {
		return []string{deltaPoint(t, 1), deltaPoint(t, 2), deltaPoint(t, 3)}, true
	})
	first.start(t)
	first.waitSignals(t, 3)
	first.stop(t)
	var mu sync.Mutex
	var sinces []string
	second := newOTLPWorld(t, []string{otlpPS("running")}, func(_ int, since string) ([]string, bool) {
		mu.Lock()
		sinces = append(sinces, since)
		mu.Unlock()
		return []string{deltaPoint(t, 1), deltaPoint(t, 2), deltaPoint(t, 3), deltaPoint(t, 4), deltaPoint(t, 5)}, true
	})
	second.s = first.s
	second.start(t)
	waitFor(t, "the lines after the cursor", func() bool {
		got := events(t, second.s)
		return len(got) >= 5 && got[len(got)-1].Metric.Value == 5
	})
	second.stop(t)
	var values []float64
	for _, sig := range events(t, second.s) {
		values = append(values, sig.Metric.Value)
	}
	mu.Lock()
	defer mu.Unlock()
	state, err := second.s.KVGet(signal.OTLPCollector)
	if err != nil {
		t.Fatal(err)
	}
	if dropped, ok := state["dropped"].(map[string]any); !ok || len(dropped) != 0 || state["cursor"] != stamp(5) {
		t.Fatalf("state %v", state)
	}
	if len(values) != 5 || values[2] != 3 || values[3] != 4 || len(sinces) != 1 || sinces[0] != stamp(3) {
		t.Fatalf("values %v sinces %v", values, sinces)
	}
}

func feed(obs *OTLP, line func(n int) string) func() int {
	ctx, cancel := context.WithCancel(context.Background())
	fed := make(chan int)
	go func() {
		n := 0
		for ctx.Err() == nil {
			n++
			obs.line(stamp(n) + " " + line(n))
			time.Sleep(20 * time.Millisecond)
		}
		fed <- n
	}()
	return func() int {
		cancel()
		return <-fed
	}
}

func TestSettleWaitsAQuietWindowFromItsStartAndStopsAtTheLimitWhileSpansKeepComing(t *testing.T) {
	trace := "w3c:" + tracectx.TraceID(runID)
	span := func(n int) string {
		return serviceSpan(t, trace, fmt.Sprintf("w3c:%016x", n), "", map[string]any{"service": "app"})
	}
	obs := &OTLP{RunID: runID, Stream: store(t, signal.DefaultCaps())}
	obs.defaults()
	obs.line(stamp(1) + " " + span(1))
	time.Sleep(200 * time.Millisecond)
	waited, quiet := obs.Settle(context.Background(), 150*time.Millisecond, 2*time.Second)
	if !quiet || waited < 150*time.Millisecond || waited > time.Second {
		t.Fatalf("after an earlier span the settle waited %s (quiet %v)", waited, quiet)
	}
	foreign := func(n int) string {
		return serviceSpan(t, "w3c:"+strings.Repeat("cd", 16), fmt.Sprintf("w3c:%016x", n), "", map[string]any{"service": "app"})
	}
	for what, line := range map[string]func(int) string{"metric lines": func(n int) string { return deltaPoint(t, n) }, "spans of a foreign trace": foreign} {
		stop := feed(obs, line)
		waited, quiet = obs.Settle(context.Background(), 150*time.Millisecond, 2*time.Second)
		if n := stop(); !quiet || waited < 150*time.Millisecond || waited > time.Second || n < 5 {
			t.Fatalf("with %d %s coming the settle waited %s (quiet %v)", n, what, waited, quiet)
		}
	}
	stop := feed(obs, span)
	waited, quiet = obs.Settle(context.Background(), 150*time.Millisecond, 400*time.Millisecond)
	if n := stop(); quiet || waited < 400*time.Millisecond || waited > 2*time.Second || n < 5 {
		t.Fatalf("with %d spans coming the settle waited %s (quiet %v)", n, waited, quiet)
	}
}

func TestACancelledFollowLeavesAPartLineForTheNextFollow(t *testing.T) {
	trace := "w3c:" + tracectx.TraceID(runID)
	whole := serviceSpan(t, trace, "w3c:0000000000000002", "", map[string]any{"service": "app"})
	first := newOTLPWorld(t, []string{otlpPS("running")}, func(int, string) ([]string, bool) {
		return []string{serviceSpan(t, trace, "w3c:0000000000000001", "", map[string]any{"service": "app"})}, true
	})
	first.d.handle = func(ctx context.Context, cmd compose.Command) (compose.Result, error) {
		fmt.Fprintf(cmd.Stdout, "%s %s\n%s %s", stamp(1), serviceSpan(t, trace, "w3c:0000000000000001", "", map[string]any{"service": "app"}), stamp(2), whole[:len(whole)/2])
		<-ctx.Done()
		return compose.Result{}, nil
	}
	first.start(t)
	first.waitSignals(t, 1)
	time.Sleep(50 * time.Millisecond)
	first.stop(t)
	state, err := first.s.KVGet(signal.OTLPCollector)
	if err != nil || state["cursor"] != stamp(1) {
		t.Fatalf("after the cut the state is %v (%v)", state, err)
	}
	second := newOTLPWorld(t, []string{otlpPS("running")}, func(int, string) ([]string, bool) {
		return []string{serviceSpan(t, trace, "w3c:0000000000000001", "", map[string]any{"service": "app"}), whole}, true
	})
	second.s = first.s
	second.start(t)
	got := byID(second.waitSignals(t, 2))
	second.stop(t)
	if _, ok := got["w3c:0000000000000002"]; !ok || len(got) != 2 {
		t.Fatalf("after the next follow the stream holds %v", got)
	}
}

func TestALineThatDockerSplitAt16KiBIsJoinedBeforeItIsParsed(t *testing.T) {
	trace := "w3c:" + tracectx.TraceID(runID)
	long := serviceSpan(t, trace, "w3c:0000000000000003", "", map[string]any{"service": "app", "pad": strings.Repeat("x", 40000)})
	w := newOTLPWorld(t, []string{otlpPS("running")}, func(int, string) ([]string, bool) { return nil, true })
	w.d.handle = func(ctx context.Context, cmd compose.Command) (compose.Result, error) {
		fmt.Fprintf(cmd.Stdout, "%s %s%s %s%s %s\n", stamp(1), long[:partialSize], stamp(1), long[partialSize:2*partialSize], stamp(1), long[2*partialSize:])
		<-ctx.Done()
		return compose.Result{}, nil
	}
	w.start(t)
	span := byID(w.waitSignals(t, 1))["w3c:0000000000000003"]
	w.stop(t)
	if pad, _ := span.Attrs["pad"].(string); len(pad) != 40000 || strings.Contains(pad, "2026") {
		t.Fatalf("the joined span has a pad of %d bytes", len(pad))
	}
	if len(long) <= 2*partialSize {
		t.Fatalf("canary: the line is %d bytes, under two parts", len(long))
	}
}

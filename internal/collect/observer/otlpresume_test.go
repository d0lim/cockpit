package observer

import (
	"context"
	"sync"
	"testing"
	"time"

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
	if len(values) != 5 || values[2] != 3 || values[3] != 4 || len(sinces) != 1 || sinces[0] != stamp(3) {
		t.Fatalf("values %v sinces %v", values, sinces)
	}
}

func TestSettleWaitsAQuietWindowFromItsStartAndStopsAtTheLimitWhileLinesKeepComing(t *testing.T) {
	obs := &OTLP{RunID: runID, Stream: store(t, signal.DefaultCaps())}
	obs.defaults()
	obs.line(stamp(1) + " not json")
	time.Sleep(200 * time.Millisecond)
	waited, quiet := obs.Settle(context.Background(), 150*time.Millisecond, 2*time.Second)
	if !quiet || waited < 150*time.Millisecond || waited > time.Second {
		t.Fatalf("after an earlier line the settle waited %s (quiet %v)", waited, quiet)
	}
	ctx, cancel := context.WithCancel(context.Background())
	fed := make(chan int)
	go func() {
		n := 0
		for ctx.Err() == nil {
			obs.line(stamp(2) + " not json")
			n++
			time.Sleep(20 * time.Millisecond)
		}
		fed <- n
	}()
	waited, quiet = obs.Settle(context.Background(), 150*time.Millisecond, 400*time.Millisecond)
	cancel()
	if n := <-fed; quiet || waited < 400*time.Millisecond || waited > 2*time.Second || n < 5 {
		t.Fatalf("with %d lines coming the settle waited %s (quiet %v)", n, waited, quiet)
	}
}

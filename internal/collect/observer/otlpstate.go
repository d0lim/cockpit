package observer

import (
	"context"
	"maps"
	"time"

	"github.com/d0lim/cockpit/internal/signal"
	"github.com/d0lim/cockpit/internal/tracectx"
)

const (
	cursorKey  = "cursor"
	droppedKey = "dropped"
	w3cPrefix  = "w3c:"
	settleTick = 50 * time.Millisecond
)

type otlpState struct {
	cursor  time.Time
	dropped map[string]int64
}

func runTraceID(runID string) string { return w3cPrefix + tracectx.TraceID(runID) }

func (o *OTLP) registerTrace(w3c, trace string) {
	if o.traces == nil {
		o.traces = map[string]string{}
	}
	o.traces[w3c] = trace
}

func (o *OTLP) TraceExec(name string) {
	o.defaults()
	o.mu.Lock()
	defer o.mu.Unlock()
	o.registerTrace(w3cPrefix+tracectx.ExecTraceID(o.RunID, name), o.RunID+"/exec/"+name)
}

func (o *OTLP) traceFor(w3c string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if trace, ok := o.traces[w3c]; ok {
		return trace
	}
	return w3c
}

func (o *OTLP) count(reason string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.state.dropped == nil {
		o.state.dropped = map[string]int64{}
	}
	o.state.dropped[reason]++
}

func (o *OTLP) load() {
	got, err := o.Stream.KVGet(signal.OTLPCollector)
	if err != nil {
		o.Log.Warn("cannot read the otlp cursor", "run", o.RunID, "err", o.Mask(err.Error()))
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if s, ok := got[cursorKey].(string); ok {
		if at, err := time.Parse(time.RFC3339Nano, s); err == nil && at.After(o.state.cursor) {
			o.state.cursor = at
		}
	}
	if counts, ok := got[droppedKey].(map[string]any); ok && o.state.dropped == nil {
		o.state.dropped = map[string]int64{}
		for reason, n := range counts {
			if f, ok := n.(float64); ok {
				o.state.dropped[reason] = int64(f)
			}
		}
	}
}

func (o *OTLP) save() {
	o.mu.Lock()
	value := map[string]any{droppedKey: maps.Clone(o.state.dropped)}
	if value[droppedKey] == nil {
		value[droppedKey] = map[string]int64{}
	}
	if !o.state.cursor.IsZero() {
		value[cursorKey] = o.state.cursor.UTC().Format(time.RFC3339Nano)
	}
	o.saved = time.Now()
	o.mu.Unlock()
	if err := o.Stream.KVPut(signal.OTLPCollector, value); err != nil {
		o.Log.Warn("cannot save the otlp cursor", "run", o.RunID, "err", o.Mask(err.Error()))
	}
}

func (o *OTLP) saveIfDue() {
	o.mu.Lock()
	due := time.Since(o.saved) >= o.SaveEvery
	o.mu.Unlock()
	if due {
		o.save()
	}
}

func (o *OTLP) Settle(ctx context.Context, quiet, limit time.Duration) (time.Duration, bool) {
	start := time.Now()
	for {
		now := time.Now()
		last := start
		if heard := time.Unix(0, o.heard.Load()); heard.After(start) {
			last = heard
		}
		if now.Sub(last) >= quiet {
			return now.Sub(start), true
		}
		if now.Sub(start) >= limit {
			return now.Sub(start), false
		}
		t := time.NewTimer(min(settleTick, quiet-now.Sub(last), limit-now.Sub(start)))
		select {
		case <-ctx.Done():
			t.Stop()
			return time.Since(start), false
		case <-t.C:
		}
	}
}

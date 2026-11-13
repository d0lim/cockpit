package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/signal"
)

func frozenOTLPTree(t *testing.T, frozen []byte, runID string) (underCalls, series int) {
	t.Helper()
	calls := map[string]bool{}
	var spans []contract.Signal
	records := map[string]int{}
	lines := 0
	for _, line := range strings.Split(strings.TrimSpace(string(frozen)), "\n") {
		var sig contract.Signal
		if err := json.Unmarshal([]byte(line), &sig); err != nil {
			t.Fatalf("frozen line %q: %v", line, err)
		}
		lines++
		switch {
		case sig.Collector == signal.RunnerSource && sig.Span != nil && sig.Span.TraceID == runID && strings.HasPrefix(sig.ID, "call:"):
			calls[sig.ID] = true
		case sig.Collector == signal.OTLPCollector && sig.Span != nil && sig.Span.TraceID == runID:
			spans = append(spans, sig)
		case sig.Collector == signal.OTLPCollector && sig.Metric != nil && sig.ID != "":
			records[sig.ID]++
		}
	}
	for _, sig := range spans {
		if calls[sig.Span.ParentID] {
			underCalls++
		}
	}
	for id, n := range records {
		if n != 1 {
			t.Errorf("the metric series %s has %d records in the frozen copy", id, n)
		}
	}
	t.Logf("frozen copy: %d lines, %d call spans, %d otlp spans in the run trace, %d under a call span, %d otlp metric series", lines, len(calls), len(spans), underCalls, len(records))
	return underCalls, len(records)
}

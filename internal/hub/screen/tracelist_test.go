package screen

import (
	"fmt"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

func TestTheTraceListPutsExecThenRunThenTheNewestTwentyServiceTracesAndKeepsAPickedOlderOne(t *testing.T) {
	spans := span1("run-1", "run", "smoke", 0)
	spans = append(spans, span1("run-1/exec/a", "exec", "health", 1)...)
	for i := range 25 {
		spans = append(spans, span1(fmt.Sprintf("w3c:%032x", i), fmt.Sprintf("w3c:%016x", i), "GET /health", 2+i)...)
	}
	traces := traceList(spans)
	if len(traces) != 27 || traces[0].id != "run-1/exec/a" || traces[1].id != "run-1" || traces[2].id != fmt.Sprintf("w3c:%032x", 24) || !traces[2].service || traces[1].service {
		t.Fatalf("order %d: %+v %+v %+v", len(traces), traces[0], traces[1], traces[2])
	}
	listed, hidden := listedTraces(traces, "run-1")
	if len(listed) != 22 || hidden != 5 || listed[21].id != fmt.Sprintf("w3c:%032x", 5) {
		t.Fatalf("listed %d hidden %d last %s", len(listed), hidden, listed[len(listed)-1].id)
	}
	old := fmt.Sprintf("w3c:%032x", 0)
	listed, hidden = listedTraces(traces, old)
	if len(listed) != 23 || hidden != 4 || listed[22].id != old {
		t.Fatalf("with the oldest picked: listed %d hidden %d last %s", len(listed), hidden, listed[len(listed)-1].id)
	}
}

func span1(trace, id, name string, second int) []contract.Signal {
	start := fmt.Sprintf("2026-10-19T01:00:%02d.000Z", second)
	return []contract.Signal{span(trace, id, "", name, start, start)}
}

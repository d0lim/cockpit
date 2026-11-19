package api

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/wire"
	"github.com/d0lim/cockpit/internal/wire/wiretest"
)

const (
	wireLinkClient = "44f067aa0ba902bb"
	wireLinkWorker = "55f067aa0ba902bc"
)

func tracedTake(t *testing.T, runDir, rootName string) {
	t.Helper()
	calls := wiretest.Calls("x")
	calls[2].Span, calls[7].Span = wireLinkClient, wireLinkWorker
	start := wiretest.T0.Add(4 * time.Second)
	spans := []wire.Span{
		wire.NewSpan(wireLinkClient, wiretest.SpanFirst, "worker", "client", nil, start),
		wire.NewSpan(wireLinkWorker, "", "worker", rootName, nil, start),
	}
	wiretest.Store(t, runDir, wiretest.Take{Name: "traced", Calls: calls, Result: wiretest.Result(), Routes: wiretest.Routes(), Spans: spans})
}

func TestTracedAndBackgroundCallsAreCountedExplainedAndCarryTheirStep(t *testing.T) {
	f, base := seededWire(t)
	tracedTake(t, f.runDir(rid(1)), "task reconcile")
	v := getWire(t, f, base+"?take=traced&flow=alpha&step=a1")
	if s := v.Steps[0]; s.Exact != 1 || s.Traced != 1 || s.Nested != 1 || s.Time != 0 || s.Calls() != 3 {
		t.Fatalf("step counts %+v", s)
	}
	outside := getWire(t, f, base+"?take=traced&scope=outside").Calls
	if len(outside) != 1 || outside[0].Link != wire.LinkBackground || outside[0].Root != "task reconcile" || outside[0].Step != "" {
		t.Fatalf("background head %+v", outside)
	}
	at := slices.IndexFunc(v.Calls, func(h WireHead) bool { return h.Seq == 3 })
	if h := v.Calls[at]; h.Link != wire.LinkTraced || h.Parent != 1 || h.Step != "a1" || h.Depth != 1 || h.Finished.IsZero() {
		t.Fatalf("traced head %+v", h)
	}
	call, _ := getWireCall(t, f, base+"/3?take=traced")
	if want := "The span chain of this call leads to call 1 (cockpit-runner to orders). It is listed under step alpha › a1."; call.Why != want {
		t.Fatalf("why %q", call.Why)
	}
}

func TestAFlowSelectionListsEveryStepOfTheFlowInStartOrder(t *testing.T) {
	f, base := seededWire(t)
	v := getWire(t, f, base+"?scope=flow&flow=alpha")
	var seqs []int64
	var steps []string
	for _, h := range v.Calls {
		seqs, steps = append(seqs, h.Seq), append(steps, h.Step)
	}
	if v.Scope != WireScopeFlow || v.Flow != "alpha" || v.Step != "" || !slices.Equal(seqs, []int64{1, 2, 3, 4, 5, 6, 7}) {
		t.Fatalf("flow view %+v", v)
	}
	if !slices.Equal(steps, []string{"a1", "a1", "a1", "a2", "a2", "a2", "a2"}) {
		t.Fatalf("steps %v", steps)
	}
	for q, want := range map[string]int{"?scope=flow": http.StatusBadRequest, "?scope=flow&flow=alpha&step=a1": http.StatusBadRequest, "?scope=flow&flow=nope": http.StatusNotFound} {
		if status, body := f.get(base + q); status != want {
			t.Errorf("%s: %d %s", q, status, body)
		}
	}
}

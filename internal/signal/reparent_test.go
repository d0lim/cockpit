package signal

import (
	"maps"
	"path/filepath"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var reparentMutations = []porttest.Mutation{
	{"a service span under a call's span id moves under the call span", "skip the Append of the moved spans in Reparent", "red: the service span keeps its w3c parent in reads and in the frozen copy", "TestAServiceSpanUnderACallSpanIDMovesUnderTheCallInReadsAndTheFrozenCopy"},
	{"the moved revision outranks the receiver's line", "keep the stored rev on the moved span", "red: the original line read again wins back its w3c parent", "TestAServiceSpanUnderACallSpanIDMovesUnderTheCallInReadsAndTheFrozenCopy"},
	{"only spans of the given trace move", "drop the trace comparison in Reparent", "red: the span of another trace moves under the call", "TestAServiceSpanUnderACallSpanIDMovesUnderTheCallInReadsAndTheFrozenCopy"},
}

func TestReparentMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "reparent_test.go", reparentMutations)
}

func serviceSpanUnder(id, trace, parent string) contract.Signal {
	return contract.Signal{
		Kind: contract.SignalKindSpan, Collector: OTLPCollector, TSSource: contract.TimeSourceContainer, ID: id, Attrs: contract.Attrs{ServiceAttr: "app"},
		Span: &contract.SpanBody{TraceID: trace, ParentID: parent, Name: "POST /orders", Start: "2026-10-12T07:00:01.110Z", End: "2026-10-12T07:00:01.120Z", Status: contract.SpanStatusOK},
	}
}

func parentsOf(signals []contract.Signal) map[string]string {
	out := map[string]string{}
	for _, sig := range signals {
		if sig.Collector == OTLPCollector {
			out[sig.Span.TraceID+" "+sig.ID] = sig.Span.ParentID
		}
	}
	return out
}

func TestAServiceSpanUnderACallSpanIDMovesUnderTheCallInReadsAndTheFrozenCopy(t *testing.T) {
	res := tracedCallResult("/orders")
	parents := CallParents(res)
	if !maps.Equal(parents, map[string]string{"w3c:00f067aa0ba902b7": "call:order/place/1"}) {
		t.Fatalf("call parents %v", parents)
	}
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	for _, sig := range RunSpans(runID, res, "") {
		mustAppend(t, s, sig)
	}
	server := serviceSpanUnder("w3c:00000000000000a1", runID, "w3c:00f067aa0ba902b7")
	mustAppend(t, s, server)
	mustAppend(t, s, serviceSpanUnder("w3c:00000000000000a2", runID, "w3c:00000000000000a1"))
	mustAppend(t, s, serviceSpanUnder("w3c:00000000000000a3", "w3c:"+runID, "w3c:00f067aa0ba902b7"))
	moved, err := Reparent(s, runID, parents)
	if err != nil || moved != 1 {
		t.Fatalf("moved %d: %v", moved, err)
	}
	mustAppend(t, s, server)
	want := map[string]string{
		runID + " w3c:00000000000000a1":          "call:order/place/1",
		runID + " w3c:00000000000000a2":          "w3c:00000000000000a1",
		"w3c:" + runID + " w3c:00000000000000a3": "w3c:00f067aa0ba902b7",
	}
	if got := parentsOf(readAll(t, s, 0).Signals); !maps.Equal(got, want) {
		t.Fatalf("live parents %v", got)
	}
	bundle := filepath.Join(t.TempDir(), "report")
	if _, err := s.Freeze(bundle); err != nil {
		t.Fatal(err)
	}
	frozen, err := ReadFrozen(bundle, masker(t), ReadRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if got := parentsOf(frozen.Signals); !maps.Equal(got, want) || len(frozen.Signals) != len(readAll(t, s, 0).Signals) {
		t.Fatalf("frozen parents %v of %d signals", got, len(frozen.Signals))
	}
	if again, err := Reparent(s, runID, parents); err != nil || again != 0 {
		t.Fatalf("a second pass moved %d: %v", again, err)
	}
}

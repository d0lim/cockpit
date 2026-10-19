package signal

import (
	"slices"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

func withID(sig contract.Signal, id string, rev int) contract.Signal {
	sig.ID, sig.Rev = id, rev
	return sig
}

func TestReemittingAnIDWithRevisionTwoIndexesARevisionAndReadsReturnOnlyTheLatest(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	mustAppend(t, s, withID(event("toy-db", "order pending", nil), "order-7", 1))
	mustAppend(t, s, metric("toy-db", 1))
	latest := mustAppend(t, s, withID(event("toy-db", "order paid", nil), "order-7", 2))
	if seq, n := s.Revisions("toy-db", contract.SignalKindEvent, "order-7"); seq != latest || n != 2 {
		t.Fatalf("revisions %d %d", seq, n)
	}
	got := readAll(t, s, 0)
	var bodies []string
	for _, sig := range got.Signals {
		if sig.Event != nil {
			bodies = append(bodies, sig.Event.Body)
		}
	}
	if !slices.Equal(bodies, []string{"order paid"}) || len(got.Signals) != 2 {
		t.Fatalf("%v %v", bodies, seqs(got))
	}
}

func TestOlderRevisionArrivingLateDoesNotReplaceTheLatest(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	mustAppend(t, s, withID(event("toy-db", "rev 3", nil), "order-7", 3))
	mustAppend(t, s, withID(event("toy-db", "rev 2", nil), "order-7", 2))
	got := readAll(t, s, 0)
	if len(got.Signals) != 1 || got.Signals[0].Event.Body != "rev 3" {
		t.Fatalf("%+v", got.Signals)
	}
}

func TestSameIDFromAnotherCollectorOrKindIsNotARevision(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	mustAppend(t, s, withID(event("toy-db", "a", nil), "x", 1))
	mustAppend(t, s, withID(event("acme-queue", "b", nil), "x", 2))
	mustAppend(t, s, withID(metric("toy-db", 1), "x", 3))
	if got := readAll(t, s, 0); len(got.Signals) != 3 {
		t.Fatalf("%v", seqs(got))
	}
}

func traced(trace, id, end string, rev int) contract.Signal {
	return contract.Signal{
		TSSource: contract.TimeSourceRunner, Kind: contract.SignalKindSpan, Collector: RunnerSource, ID: id, Rev: rev,
		Span: &contract.SpanBody{TraceID: trace, Name: id, Start: "2026-10-19T01:00:00.000Z", End: end, Status: contract.SpanStatusOK},
	}
}

func TestASpanIDIsARevisionOnlyWithinItsTraceAndReplaysTheSame(t *testing.T) {
	dir := runDir(t)
	s := openAt(t, dir, DefaultCaps(), masker(t))
	mustAppend(t, s, traced("run-1", "flow:health", "2026-10-19T01:00:01.000Z", 0))
	mustAppend(t, s, traced("run-1/exec/a", "flow:health", "2026-10-19T01:00:02.000Z", 0))
	mustAppend(t, s, traced("run-1/exec/b", "flow:health", "2026-10-19T01:00:03.000Z", 0))
	mustAppend(t, s, traced("run-1", "flow:health", "2026-10-19T01:00:04.000Z", 1))
	ends := func(s *Stream) []string {
		var out []string
		for _, sig := range readAll(t, s, 0).Signals {
			out = append(out, sig.Span.TraceID+" "+sig.Span.End)
		}
		return out
	}
	want := []string{"run-1/exec/a 2026-10-19T01:00:02.000Z", "run-1/exec/b 2026-10-19T01:00:03.000Z", "run-1 2026-10-19T01:00:04.000Z"}
	if got := ends(s); !slices.Equal(got, want) {
		t.Fatalf("live %v", got)
	}
	s.Close()
	if got := ends(openAt(t, dir, DefaultCaps(), masker(t))); !slices.Equal(got, want) {
		t.Fatalf("replayed %v", got)
	}
}

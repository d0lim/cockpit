package tracectx

import (
	"strings"
	"testing"
)

const runID = "run-20261012-153001-997a37e"

func TestTheTraceIDIsStableForARunAndDiffersBetweenRuns(t *testing.T) {
	id := TraceID(runID)
	if id != "529345c60c6eb3439cf5567c6e3753c0" || TraceID(runID) != id || !IsTraceID(id) {
		t.Fatalf("trace id %q is not the first 16 bytes of sha256(cockpit/<run id>)", id)
	}
	if other := TraceID("run-20261012-153002-0b1c2d3"); other == id || !IsTraceID(other) {
		t.Fatalf("another run got %q", other)
	}
	exec := ExecTraceID(runID, "2026-10-13T01-00-00-000Z-health")
	if exec != "6d9a7f8d636aae213cb5bb9d91453a86" || exec == id || ExecTraceID(runID, "2026-10-13T01-00-00-000Z-other") == exec {
		t.Fatalf("exec trace id %q", exec)
	}
}

func TestATraceparentRoundTripsThroughFormatAndParse(t *testing.T) {
	trace, span := TraceID(runID), "00f067aa0ba902b7"
	header := Format(trace, span)
	if header != "00-"+trace+"-00f067aa0ba902b7-01" {
		t.Fatalf("format %q", header)
	}
	valid := map[string][2]string{
		header: {trace, span},
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00":          {"4bf92f3577b34da6a3ce929d0e0e4736", span},
		"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-extended": {"4bf92f3577b34da6a3ce929d0e0e4736", span},
	}
	for in, want := range valid {
		gotTrace, gotSpan, ok := Parse(in)
		if !ok || gotTrace != want[0] || gotSpan != want[1] {
			t.Fatalf("%q parsed as %q %q %v", in, gotTrace, gotSpan, ok)
		}
	}
	good := "4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	malformed := []string{
		"",
		"00",
		"00-" + good[:len(good)-1],
		"00-" + good + "-",
		"00-" + good + "x",
		"01-" + good + "x",
		"ff-" + good,
		"0g-" + good,
		"0A-" + good,
		"00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01",
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00F067AA0BA902B7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-0z",
		"00_4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736_00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7_01",
		" 00-" + good,
		strings.Repeat("x", 55),
	}
	rejected := 0
	for _, in := range malformed {
		if gotTrace, gotSpan, ok := Parse(in); ok || gotTrace != "" || gotSpan != "" {
			t.Errorf("%q parsed as %q %q", in, gotTrace, gotSpan)
			continue
		}
		rejected++
	}
	if rejected != 19 || len(malformed) != 19 {
		t.Fatalf("rejected %d of %d malformed values, want 19", rejected, len(malformed))
	}
}

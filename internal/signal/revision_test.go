package signal

import (
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var revisionMutations = []porttest.Mutation{
	{"a re-emitted id supersedes the earlier winner", "admit every record in record instead of noting its revision", "red: both revisions of order-7 are read", "TestReemittingAnIDWithRevisionTwoIndexesARevisionAndReadsReturnOnlyTheLatest"},
	{"a lower revision never replaces the winner", "let note take any rev as the winner", "red: the late rev 2 is read instead of rev 3", "TestOlderRevisionArrivingLateDoesNotReplaceTheLatest"},
	{"the revision key holds the collector and the kind", "build revKey from the id alone", "red: the same id from another collector or kind collapses to one record", "TestSameIDFromAnotherCollectorOrKindIsNotARevision"},
	{"a span revision stays inside its trace", "leave the trace id out of sigKey", "red: the exec spans with the same id supersede each other", "TestASpanIDIsARevisionOnlyWithinItsTraceAndReplaysTheSame"},
	{"a metric with an id is a revision", "restore the metric exclusion in record", "red: the series reads back as six values instead of the rev 5 total", "TestAMetricWithAnIDKeepsOnlyItsHighestRevision"},
	{"a metric without an id stays append only", "treat an empty id as an id in record", "red: five points without an id read back as one", "TestAMetricWithoutAnIDStaysAppendOnly"},
	{"a metric body keeps its declared shape", "skip ValidateMetric in validate", "red: every malformed histogram is accepted", "TestAHistogramMetricIsStoredWithItsShapeAndAMalformedOneIsRefused"},
}

func TestRevisionMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "revision_test.go", revisionMutations)
}

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

func series(id string, rev int, v float64) contract.Signal {
	sig := withID(metric("toy-db", v), id, rev)
	sig.Attrs = contract.Attrs{"table": "series"}
	return sig
}

func metricValues(resp ReadResponse) []float64 {
	var out []float64
	for _, sig := range resp.Signals {
		if sig.Metric != nil {
			out = append(out, sig.Metric.Value)
		}
	}
	return out
}

func TestAMetricWithAnIDKeepsOnlyItsHighestRevision(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	for rev := 1; rev <= 5; rev++ {
		mustAppend(t, s, series("requests", rev, float64(rev*10)))
	}
	mustAppend(t, s, series("requests", 3, 99))
	if got := metricValues(readAll(t, s, 0)); !slices.Equal(got, []float64{50}) {
		t.Fatalf("values %v", got)
	}
	if seq, n := s.Revisions("toy-db", contract.SignalKindMetric, "requests"); seq != 5 || n != 6 {
		t.Fatalf("revisions %d %d", seq, n)
	}
}

func TestAMetricWithoutAnIDStaysAppendOnly(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	for v := 1; v <= 5; v++ {
		mustAppend(t, s, metric("toy-db", float64(v)))
		mustAppend(t, s, series("requests", v, float64(v)))
	}
	got := metricValues(readAll(t, s, 0))
	if !slices.Equal(got, []float64{1, 2, 3, 4, 5, 5}) {
		t.Fatalf("values %v", got)
	}
}

func histogramMetric(mutate func(*contract.MetricBody)) contract.Signal {
	sig := metric("toy-db", 0.6)
	sig.Metric.Name, sig.Metric.Unit, sig.Metric.Type = "latency", "s", contract.MetricTypeHistogram
	sig.Metric.Histogram = &contract.HistogramBody{Count: 3, Sum: 0.6, Bounds: []float64{0.1, 0.5}, Counts: []uint64{1, 1, 1}}
	mutate(sig.Metric)
	return sig
}

func TestAHistogramMetricIsStoredWithItsShapeAndAMalformedOneIsRefused(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	mustAppend(t, s, histogramMetric(func(*contract.MetricBody) {}))
	mustAppend(t, s, histogramMetric(func(m *contract.MetricBody) { m.Histogram.Bounds, m.Histogram.Counts = nil, nil }))
	malformed := map[string]func(*contract.MetricBody){
		"exactly when":   func(m *contract.MetricBody) { m.Type = contract.MetricTypeSum },
		"exactly when ":  func(m *contract.MetricBody) { m.Histogram = nil },
		"equal its sum":  func(m *contract.MetricBody) { m.Value = 1 },
		"one count more": func(m *contract.MetricBody) { m.Histogram.Counts = []uint64{3} },
		"must ascend":    func(m *contract.MetricBody) { m.Histogram.Bounds = []float64{0.5, 0.1} },
		"unknown metric": func(m *contract.MetricBody) { m.Type = "rate" },
	}
	refused := 0
	for want, mutate := range malformed {
		if _, err := s.Append(histogramMetric(mutate)); err == nil || !strings.Contains(err.Error(), strings.TrimSpace(want)) {
			t.Errorf("%s: %v", want, err)
			continue
		}
		refused++
	}
	got := readAll(t, s, 0).Signals
	if refused != 6 || len(got) != 2 {
		t.Fatalf("refused %d, stored %d", refused, len(got))
	}
	m := got[0].Metric
	if m.Unit != "s" || m.Type != contract.MetricTypeHistogram || m.Histogram == nil || m.Histogram.Count != 3 || m.Histogram.Sum != 0.6 ||
		!slices.Equal(m.Histogram.Bounds, []float64{0.1, 0.5}) || !slices.Equal(m.Histogram.Counts, []uint64{1, 1, 1}) {
		t.Fatalf("stored %+v %+v", m, m.Histogram)
	}
}

package otlp

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

func writeOne(t *testing.T, s Signal) (Signal, int) {
	t.Helper()
	var out bytes.Buffer
	w, err := NewWriter(&out)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Write([]Signal{s}); err != nil {
		t.Fatal(err)
	}
	r := &receiver{out: &out}
	lines := r.lines(t)
	if len(lines) != 1 {
		t.Fatalf("%d lines, want 1", len(lines))
	}
	return lines[0].Signal, out.Len()
}

func manyAttrs(n int) contract.Attrs {
	attrs := contract.Attrs{ServiceAttr: "toy-api"}
	for i := range n {
		attrs[fmt.Sprintf("a%03d", i)] = strings.Repeat("v", MaxString)
	}
	return attrs
}

func TestALineOverTheLimitDropsAttributesThenLinksThenBuckets(t *testing.T) {
	links := make([]contract.SpanLink, 2000)
	for i := range links {
		links[i] = contract.SpanLink{TraceID: "w3c:" + traceHex, SpanID: "w3c:" + rootHex}
	}
	span := Signal{Kind: contract.SignalKindSpan, TS: "t", TSSource: contract.TimeSourceContainer, Attrs: manyAttrs(100), Span: &contract.SpanBody{TraceID: "w3c:" + traceHex, Name: "n", Start: "t", Status: contract.SpanStatusOK, Links: links[:10]}}
	got, n := writeOne(t, span)
	if n >= MaxLine || len(got.Attrs) != 2 || got.Attrs[ServiceAttr] != "toy-api" || got.Attrs["attrsDropped"] != float64(100) || len(got.Span.Links) != 10 {
		t.Fatalf("%d bytes with attrs %v and %d links, want the attributes dropped and the links kept", n, got.Attrs, len(got.Span.Links))
	}
	span.Attrs, span.Span.Links = manyAttrs(1), links
	got, n = writeOne(t, span)
	if n >= MaxLine || got.Span.Links != nil || got.Attrs["attrsDropped"] != float64(1) {
		t.Fatalf("%d bytes with %d links, want the links dropped after the attributes", n, len(got.Span.Links))
	}
	bounds, counts := make([]float64, 10000), make([]uint64, 10001)
	for i := range bounds {
		bounds[i] = float64(i) + 0.123456789
	}
	metric := Signal{Kind: contract.SignalKindMetric, TS: "t", TSSource: contract.TimeSourceContainer, Attrs: contract.Attrs{ServiceAttr: "toy-api"}, Metric: &contract.MetricBody{Name: "m", Type: contract.MetricTypeHistogram, Histogram: &contract.HistogramBody{Count: 1, Sum: 2, Bounds: bounds, Counts: counts}}}
	got, n = writeOne(t, metric)
	if n >= MaxLine || got.Metric.Histogram.Bounds != nil || got.Metric.Histogram.Count != 1 || got.Metric.Histogram.Sum != 2 {
		t.Fatalf("%d bytes with histogram %d bounds, want count and sum only", n, len(got.Metric.Histogram.Bounds))
	}
	small := Signal{Kind: contract.SignalKindMetric, TS: "t", TSSource: contract.TimeSourceContainer, Attrs: manyAttrs(3), Metric: &contract.MetricBody{Name: "m", Type: contract.MetricTypeHistogram, Histogram: &contract.HistogramBody{Count: 1, Sum: 2, Bounds: []float64{1}, Counts: []uint64{0, 1}}}}
	if got, _ = writeOne(t, small); len(got.Attrs) != 4 || len(got.Metric.Histogram.Bounds) != 1 {
		t.Fatalf("a line under the limit lost data: %v %+v", got.Attrs, got.Metric.Histogram)
	}
}

func TestALargeExportKeepsItsSpansInOrder(t *testing.T) {
	r := newReceiver(t)
	spans := make([]*tracev1.Span, 1000)
	for i := range spans {
		spans[i] = &tracev1.Span{TraceId: bytesOf(t, traceHex), SpanId: bytesOf(t, fmt.Sprintf("%016x", i+1)), Name: fmt.Sprint(i), StartTimeUnixNano: 1}
	}
	if got := r.postProto(t, "/v1/traces", traces("toy-api", spans...)); got.status != 200 {
		t.Fatalf("status %d", got.status)
	}
	lines := r.lines(t)
	if len(lines) != len(spans) {
		t.Fatalf("%d lines, want %d", len(lines), len(spans))
	}
	for i, l := range lines {
		if l.Signal.Span.Name != fmt.Sprint(i) || l.Signal.ID != fmt.Sprintf("w3c:%016x", i+1) {
			t.Fatalf("line %d holds span %q %q, want the export order", i, l.Signal.Span.Name, l.Signal.ID)
		}
	}
}

package otlp

import (
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

var redactionMutations = []porttest.Mutation{
	{"attribute redaction", "redact keeps s.Attrs without calling Leaves", "red: the authorization, cookie and metric attributes keep PLANTED-TOKEN-0001, PLANTEDSESSION and PLANTED-TOKEN-0003", "TestAPlantedSecretInAnAttributeOrANameIsMaskedNextToACanaryThatPassesUnchanged"},
	{"name redaction", "text cuts without calling Mask", "red: the span name and the metric name keep PLANTED-TOKEN-0002 and PLANTED-TOKEN-0004", "TestAPlantedSecretInAnAttributeOrANameIsMaskedNextToACanaryThatPassesUnchanged"},
	{"credential key hides the whole value", "redact masks each attribute value with Mask instead of Leaves", "red: the cookie attribute keeps PLANTEDSESSION", "TestAPlantedSecretInAnAttributeOrANameIsMaskedNextToACanaryThatPassesUnchanged"},
	{"mask before cut", "cut each attribute value before Leaves masks it", "red: the cut value ends in Bearer PLANT, a fragment too short for the pattern", "TestAStringIsMaskedBeforeItIsCut"},
	{"attribute cut", "cutAll returns every value unchanged", "red: the 10 KiB attribute stays 10240 bytes", "TestAStringIsMaskedBeforeItIsCut"},
}

func TestRedactionMutationRecordMatchesTheTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "redaction_test.go", redactionMutations)
}

func TestAPlantedSecretInAnAttributeOrANameIsMaskedNextToACanaryThatPassesUnchanged(t *testing.T) {
	r := newReceiver(t)
	span := &tracev1.Span{
		TraceId: bytesOf(t, traceHex), SpanId: bytesOf(t, rootHex), StartTimeUnixNano: 1,
		Name: "GET /items Bearer PLANTED-TOKEN-0002",
		Attributes: []*commonv1.KeyValue{
			str("authorization", "Bearer PLANTED-TOKEN-0001"),
			str("http.request.header.cookie", "PLANTEDSESSION"),
			str("http.route", "/items"),
		},
	}
	if got := r.postProto(t, "/v1/traces", traces("toy-api", span)); got.status != 200 {
		t.Fatalf("traces: status %d", got.status)
	}
	md := metricExport(&metricsv1.Metric{Name: "toy.calls token=PLANTED-TOKEN-0004", Data: &metricsv1.Metric_Gauge{Gauge: &metricsv1.Gauge{
		DataPoints: []*metricsv1.NumberDataPoint{{TimeUnixNano: 1, Attributes: []*commonv1.KeyValue{str("note", "token=PLANTED-TOKEN-0003"), str("route", "/items")}, Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 1}}},
	}}})
	if got := r.postProto(t, "/v1/metrics", md); got.status != 200 {
		t.Fatalf("metrics: status %d", got.status)
	}
	lines := r.lines(t)
	if len(lines) != 2 {
		t.Fatalf("%d lines, want 2", len(lines))
	}
	s, m := lines[0].Signal, lines[1].Signal
	masked := map[string]string{
		"authorization attribute": s.Attrs["authorization"].(string),
		"cookie attribute":        s.Attrs["http.request.header.cookie"].(string),
		"span name":               s.Span.Name,
		"metric attribute":        m.Attrs["note"].(string),
		"metric name":             m.Metric.Name,
	}
	for where, v := range masked {
		if strings.Contains(v, "PLANTED") || !strings.Contains(v, "****") {
			t.Errorf("%s reads %q, want **** in place of the planted value", where, v)
		}
	}
	if len(masked) != 5 {
		t.Fatalf("checked %d values, want 5", len(masked))
	}
	if s.Attrs["http.route"] != "/items" || m.Attrs["route"] != "/items" || s.ID != "w3c:"+rootHex || s.Span.TraceID != "w3c:"+traceHex || !strings.HasPrefix(m.ID, "metric:") {
		t.Fatalf("canaries changed: %v %v %q %q %q", s.Attrs, m.Attrs, s.ID, s.Span.TraceID, m.ID)
	}
	if strings.Contains(r.out.String(), "PLANTED") || strings.Contains(r.log.String(), "PLANTED") {
		t.Fatal("a planted value reached stdout or stderr")
	}
}

func TestAStringIsMaskedBeforeItIsCut(t *testing.T) {
	r := newReceiver(t)
	edge := strings.Repeat("a", MaxString-13) + " Bearer PLANTEDTOKENVALUE"
	long := strings.Repeat("é", 5*1024)
	span := &tracev1.Span{
		TraceId: bytesOf(t, traceHex), SpanId: bytesOf(t, rootHex), StartTimeUnixNano: 1, Name: "GET /items",
		Attributes: []*commonv1.KeyValue{str("edge", edge), str("long", long)},
	}
	if got := r.postProto(t, "/v1/traces", traces("toy-api", span)); got.status != 200 {
		t.Fatalf("status %d", got.status)
	}
	lines := r.lines(t)
	if len(lines) != 1 {
		t.Fatalf("%d lines, want 1", len(lines))
	}
	got := lines[0].Signal.Attrs["edge"].(string)
	if strings.Contains(got, "PLANT") || !strings.HasSuffix(got, " Bearer ****") {
		t.Fatalf("value near the cut ends in %q, want Bearer **** with no fragment of the planted value", got[len(got)-24:])
	}
	cutValue := lines[0].Signal.Attrs["long"].(string)
	if len(cutValue) > MaxString || len(cutValue) < MaxString-1 || !strings.HasPrefix(long, cutValue) {
		t.Fatalf("a %d byte value was cut to %d bytes, want at most %d on a character boundary", len(long), len(cutValue), MaxString)
	}
}

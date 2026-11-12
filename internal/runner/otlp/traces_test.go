package otlp

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

const (
	traceHex  = "4bf92f3577b34da6a3ce929d0e0e4736"
	rootHex   = "00f067aa0ba902b7"
	childHex  = "b7ad6b7169203331"
	linkedHex = "53995c3f42cd8ad8"
)

func spanPair(t *testing.T) *tracev1.TracesData {
	t.Helper()
	root := &tracev1.Span{
		TraceId: bytesOf(t, traceHex), SpanId: bytesOf(t, rootHex), Name: "GET /items",
		StartTimeUnixNano: 1_794_457_309_000_000_000, EndTimeUnixNano: 1_794_457_309_250_000_000,
		Attributes: []*commonv1.KeyValue{str("http.route", "/items"), str("service", "spoofed")},
	}
	child := &tracev1.Span{
		TraceId: bytesOf(t, traceHex), SpanId: bytesOf(t, childHex), ParentSpanId: bytesOf(t, rootHex), Name: "SELECT items",
		StartTimeUnixNano: 1_794_457_309_100_000_000, EndTimeUnixNano: 1_794_457_309_200_000_000,
		Status: &tracev1.Status{Code: tracev1.Status_STATUS_CODE_ERROR},
		Links:  []*tracev1.Span_Link{{TraceId: bytesOf(t, traceHex), SpanId: bytesOf(t, linkedHex)}},
	}
	return traces("toy-api", root, child)
}

func TestAProtobufTraceExportBecomesOneLinePerSpan(t *testing.T) {
	r := newReceiver(t)
	got := r.postProto(t, "/v1/traces", spanPair(t))
	if got.status != 200 || got.ctype != contentProtobuf || len(got.body) != 0 {
		t.Fatalf("answer %d %q %q, want 200 with an empty protobuf body", got.status, got.ctype, got.body)
	}
	lines := r.lines(t)
	if len(lines) != 2 {
		t.Fatalf("%d lines, want one per span", len(lines))
	}
	root, child := lines[0], lines[1]
	for _, l := range lines {
		if l.V != 1 || l.Signal.Kind != contract.SignalKindSpan || l.Signal.TSSource != contract.TimeSourceContainer {
			t.Fatalf("line %+v does not carry version 1, kind span and the container time source", l)
		}
		if l.Signal.Span.TraceID != "w3c:"+traceHex || l.Signal.Attrs[ServiceAttr] != "toy-api" {
			t.Fatalf("line %+v lost the trace id or the service", l.Signal)
		}
	}
	if root.Signal.ID != "w3c:"+rootHex || root.Signal.Span.ParentID != "" || root.Signal.Span.Status != contract.SpanStatusOK {
		t.Fatalf("root span %+v", root.Signal.Span)
	}
	if root.Signal.TS != "2026-11-12T04:21:49Z" || root.Signal.Span.Start != root.Signal.TS || root.Signal.Span.End != "2026-11-12T04:21:49.25Z" {
		t.Fatalf("root span times %q %q %q", root.Signal.TS, root.Signal.Span.Start, root.Signal.Span.End)
	}
	if root.Signal.Attrs["http.route"] != "/items" {
		t.Fatalf("root span attributes %v", root.Signal.Attrs)
	}
	if child.Signal.ID != "w3c:"+childHex || child.Signal.Span.ParentID != "w3c:"+rootHex || child.Signal.Span.Status != contract.SpanStatusError {
		t.Fatalf("child span %+v", child.Signal.Span)
	}
	if links := child.Signal.Span.Links; len(links) != 1 || links[0] != (contract.SpanLink{TraceID: "w3c:" + traceHex, SpanID: "w3c:" + linkedHex}) {
		t.Fatalf("child links %+v", links)
	}
}

func TestAJSONTraceExportKeepsHexIDs(t *testing.T) {
	r := newReceiver(t)
	body := fmt.Sprintf(`{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"toy-api"}}]},
		"scopeSpans":[{"spans":[{"traceId":%q,"spanId":%q,"parentSpanId":%q,"name":"GET /items",
		"startTimeUnixNano":"1794457309000000000","endTimeUnixNano":"1794457309250000000",
		"links":[{"traceId":%q,"spanId":%q}],"unknownField":true}]}]}]}`, traceHex, childHex, rootHex, traceHex, linkedHex)
	got := r.post(t, "/v1/traces", []byte(body), "Content-Type", "application/json; charset=utf-8")
	if got.status != 200 || got.ctype != contentJSON || string(got.body) != "{}" {
		t.Fatalf("answer %d %q %q, want 200 with {}", got.status, got.ctype, got.body)
	}
	lines := r.lines(t)
	if len(lines) != 1 {
		t.Fatalf("%d lines, want 1", len(lines))
	}
	s := lines[0].Signal
	if s.ID != "w3c:"+childHex || s.Span.TraceID != "w3c:"+traceHex || s.Span.ParentID != "w3c:"+rootHex {
		t.Fatalf("ids %q %q %q, want the hex ids that were sent", s.ID, s.Span.TraceID, s.Span.ParentID)
	}
	if len(s.Span.Links) != 1 || s.Span.Links[0].SpanID != "w3c:"+linkedHex {
		t.Fatalf("links %+v, want the hex link id that was sent", s.Span.Links)
	}
}

func TestAJSONTraceExportWithAnIDThatIsNotHexOrBase64IsRefused(t *testing.T) {
	r := newReceiver(t)
	body := `{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"not hex!","spanId":"` + rootHex + `"}]}]}]}`
	if got := r.post(t, "/v1/traces", []byte(body), "Content-Type", contentJSON); got.status != 400 {
		t.Fatalf("status %d, want 400", got.status)
	}
	if got := r.post(t, "/v1/traces", []byte(`{"resourceSpans":`), "Content-Type", contentJSON); got.status != 400 {
		t.Fatalf("broken JSON: status %d, want 400", got.status)
	}
	if r.out.Len() != 0 {
		t.Fatalf("a refused export wrote %q", r.out.String())
	}
}

func TestAGzipBodyDecodes(t *testing.T) {
	r := newReceiver(t)
	got := r.post(t, "/v1/traces", fixture(t, "sdk-traces.gz.bin"), "Content-Type", contentProtobuf, "Content-Encoding", "gzip")
	if got.status != 200 {
		t.Fatalf("status %d, want 200", got.status)
	}
	lines := r.lines(t)
	if len(lines) != 2 {
		t.Fatalf("%d lines from the SDK export, want 2", len(lines))
	}
	byName := map[string]Signal{}
	for _, l := range lines {
		byName[l.Signal.Span.Name] = l.Signal
	}
	server, query := byName["GET /items"], byName["SELECT items"]
	if server.Span == nil || query.Span == nil {
		t.Fatalf("spans %v, want GET /items and SELECT items", byName)
	}
	if server.Span.ParentID != "w3c:"+rootHex || query.Span.ParentID != server.ID || server.Span.TraceID != "w3c:"+traceHex {
		t.Fatalf("parents %q %q and trace %q, want the remote parent and the server span", server.Span.ParentID, query.Span.ParentID, server.Span.TraceID)
	}
	if query.Attrs["db.system"] != "postgresql" || query.Attrs["db.cached"] != false || server.Attrs[ServiceAttr] != "toy-api" {
		t.Fatalf("attributes %v %v", server.Attrs, query.Attrs)
	}
	var zipped bytes.Buffer
	zw := gzip.NewWriter(&zipped)
	zw.Write([]byte(`{"resourceSpans":[]}`))
	zw.Close()
	if got := r.post(t, "/v1/traces", zipped.Bytes(), "Content-Type", contentJSON, "Content-Encoding", "gzip"); got.status != 200 {
		t.Fatalf("gzip JSON: status %d, want 200", got.status)
	}
	if got := r.post(t, "/v1/traces", []byte("not gzip"), "Content-Type", contentProtobuf, "Content-Encoding", "gzip"); got.status != 400 {
		t.Fatalf("bad gzip: status %d, want 400", got.status)
	}
	if !strings.Contains(r.log.String(), `"status":400`) {
		t.Fatalf("the refused request is not logged: %q", r.log.String())
	}
}

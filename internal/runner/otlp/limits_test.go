package otlp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

var limitMutations = []porttest.Mutation{
	{"signal cap", "spanSignals and metricSignals skip the MaxSignals check", "red: the trace export of 50001 spans is answered 200 with an empty body", "TestAnExportOverTheSignalCapIsRefusedWith413AndWritesNoLine"},
	{"chunked lines", "Write builds the whole export as one chunk (each chunkSignals in Write becomes len(sigs))", "red: the live heap grows by 41832120 bytes while writing 15073280 bytes in 1 write", "TestWritingAnExportHoldsLessThanAQuarterOfItsOutputInMemory"},
	{"chunked lines", "Write builds the whole export as one chunk (each chunkSignals in Write becomes len(sigs))", "red: the export of 3079 spans arrives in 1 write, want 4", "TestALargeExportIsWrittenInChunksOfAtMostChunkSignalsLines"},
	{"one export at a time", "Handler.export does not lock h.exports", "red: the export over the cap is answered 413 while the export before it is still being written", "TestAnExportWaitsUntilTheExportBeforeItIsWritten"},
	{"one export at a time", "Handler.export does not lock h.exports", "red: the race detector reports 7 data races and 7175 whole lines arrive, want 8200", "TestConcurrentExportsLargerThanAChunkNeverInterleaveTheirLines"},
}

func TestLimitMutationRecordMatchesTheTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "limits_test.go", limitMutations)
}

func emptySpans(n int) []*tracev1.Span {
	spans := make([]*tracev1.Span, n)
	for i := range spans {
		spans[i] = &tracev1.Span{}
	}
	return spans
}

func namedSpans(t *testing.T, service string, n int) *tracev1.TracesData {
	t.Helper()
	spans := make([]*tracev1.Span, n)
	for i := range spans {
		spans[i] = &tracev1.Span{TraceId: bytesOf(t, traceHex), SpanId: bytesOf(t, fmt.Sprintf("%016x", i+1)), Name: fmt.Sprint(i), StartTimeUnixNano: 1}
	}
	return traces(service, spans...)
}

func gaugePoints(n int) *metricsv1.MetricsData {
	points := make([]*metricsv1.NumberDataPoint, n)
	for i := range points {
		points[i] = &metricsv1.NumberDataPoint{TimeUnixNano: 1, Value: &metricsv1.NumberDataPoint_AsInt{AsInt: int64(i)}}
	}
	return metricExport(&metricsv1.Metric{Name: "toy.load", Data: &metricsv1.Metric_Gauge{Gauge: &metricsv1.Gauge{DataPoints: points}}})
}

func TestAnExportOverTheSignalCapIsRefusedWith413AndWritesNoLine(t *testing.T) {
	r := newReceiver(t)
	for path, export := range map[string]any{"/v1/traces": traces("toy-api", emptySpans(MaxSignals+1)...), "/v1/metrics": gaugePoints(MaxSignals + 1)} {
		var got answer
		switch m := export.(type) {
		case *tracev1.TracesData:
			got = r.postProto(t, path, m)
		case *metricsv1.MetricsData:
			got = r.postProto(t, path, m)
		}
		if got.status != http.StatusRequestEntityTooLarge || !strings.Contains(string(got.body), "more than 50000") {
			t.Fatalf("%s with %d signals: status %d body %q, want 413 with the reason", path, MaxSignals+1, got.status, got.body)
		}
	}
	if r.out.Len() != 0 {
		t.Fatalf("refused exports wrote %d bytes", r.out.Len())
	}
	if got := r.postProto(t, "/v1/metrics", gaugePoints(MaxSignals)); got.status != http.StatusOK {
		t.Fatalf("an export of exactly %d points: status %d, want 200", MaxSignals, got.status)
	}
	if n := len(r.lines(t)); n != MaxSignals {
		t.Fatalf("an export at the cap wrote %d lines, want %d", n, MaxSignals)
	}
}

type heapProbe struct {
	peak   uint64
	bytes  int
	writes []int
}

func (p *heapProbe) Write(b []byte) (int, error) {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	p.peak = max(p.peak, m.HeapAlloc)
	p.bytes += len(b)
	p.writes = append(p.writes, bytes.Count(b, []byte{'\n'}))
	return len(b), nil
}

func TestWritingAnExportHoldsLessThanAQuarterOfItsOutputInMemory(t *testing.T) {
	attrs := contract.Attrs{ServiceAttr: "toy-api", "payload": strings.Repeat("p", 128)}
	body := &contract.SpanBody{TraceID: "w3c:" + traceHex, Name: "GET /items", Start: "t", Status: contract.SpanStatusOK}
	sigs := make([]Signal, 40*chunkSignals)
	for i := range sigs {
		sigs[i] = Signal{Kind: contract.SignalKindSpan, ID: "w3c:" + rootHex, TS: "t", TSSource: contract.TimeSourceContainer, Attrs: attrs, Span: body}
	}
	probe := &heapProbe{}
	w, err := NewWriter(probe)
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	if err := w.Write(sigs); err != nil {
		t.Fatal(err)
	}
	if probe.bytes < len(sigs)*128 {
		t.Fatalf("the export wrote %d bytes, want at least the %d payload bytes", probe.bytes, len(sigs)*128)
	}
	growth := int64(probe.peak) - int64(before.HeapAlloc)
	if growth >= int64(probe.bytes/4) {
		t.Fatalf("the live heap grew by %d bytes while writing %d bytes in %d writes, want under a quarter of the output", growth, probe.bytes, len(probe.writes))
	}
}

func TestALargeExportIsWrittenInChunksOfAtMostChunkSignalsLines(t *testing.T) {
	r := newReceiver(t)
	probe := &heapProbe{}
	var out bytes.Buffer
	w, err := NewWriter(io.MultiWriter(&out, probe))
	if err != nil {
		t.Fatal(err)
	}
	r.srv.Config.Handler.(*Handler).Out = w
	n := 3*chunkSignals + 7
	if got := r.postProto(t, "/v1/traces", namedSpans(t, "toy-api", n)); got.status != http.StatusOK {
		t.Fatalf("status %d", got.status)
	}
	if len(probe.writes) != 4 {
		t.Fatalf("the export of %d spans arrived in %d writes %v, want 4", n, len(probe.writes), probe.writes)
	}
	for i, lines := range probe.writes {
		if lines > chunkSignals {
			t.Fatalf("write %d carried %d lines, want at most %d", i, lines, chunkSignals)
		}
	}
	lines := (&receiver{out: &out}).lines(t)
	if len(lines) != n {
		t.Fatalf("%d lines, want %d", len(lines), n)
	}
	for i, l := range lines {
		if l.Signal.Span.Name != fmt.Sprint(i) {
			t.Fatalf("line %d holds span %q, want the export order", i, l.Signal.Span.Name)
		}
	}
}

type heldWriter struct {
	entered, release chan struct{}
	once             sync.Once
}

func (h *heldWriter) Write(b []byte) (int, error) {
	h.once.Do(func() {
		close(h.entered)
		<-h.release
	})
	return len(b), nil
}

func TestAnExportWaitsUntilTheExportBeforeItIsWritten(t *testing.T) {
	r := newReceiver(t)
	held := &heldWriter{entered: make(chan struct{}), release: make(chan struct{})}
	w, err := NewWriter(held)
	if err != nil {
		t.Fatal(err)
	}
	r.srv.Config.Handler.(*Handler).Out = w
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(held.release) }) })
	first, second := make(chan answer, 1), make(chan answer, 1)
	go func() { first <- r.postProto(t, "/v1/traces", spanPair(t)) }()
	<-held.entered
	go func() { second <- r.postProto(t, "/v1/traces", traces("toy-api", emptySpans(MaxSignals+1)...)) }()
	select {
	case got := <-second:
		t.Fatalf("the second export was answered %d while the first was still being written", got.status)
	case <-time.After(300 * time.Millisecond):
	}
	release.Do(func() { close(held.release) })
	if got := <-second; got.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("the second export: status %d, want 413 once the first is written", got.status)
	}
	if got := <-first; got.status != http.StatusOK {
		t.Fatalf("the first export: status %d, want 200", got.status)
	}
}

func TestConcurrentExportsLargerThanAChunkNeverInterleaveTheirLines(t *testing.T) {
	r := newReceiver(t)
	const exports, per = 8, chunkSignals + 1
	var wg sync.WaitGroup
	for e := range exports {
		wg.Go(func() {
			if got := r.postProto(t, "/v1/traces", namedSpans(t, fmt.Sprintf("svc-%d", e), per)); got.status != http.StatusOK {
				t.Errorf("status %d", got.status)
			}
		})
	}
	wg.Wait()
	lines := r.lines(t)
	if len(lines) != exports*per {
		t.Fatalf("%d lines, want %d whole lines", len(lines), exports*per)
	}
	for start := 0; start < len(lines); start += per {
		service := lines[start].Signal.Attrs[ServiceAttr]
		for i, l := range lines[start : start+per] {
			if l.Signal.Attrs[ServiceAttr] != service || l.Signal.Span.Name != fmt.Sprint(i) {
				t.Fatalf("line %d holds %v span %q inside the export of %v, want each export whole and in order", start+i, l.Signal.Attrs[ServiceAttr], l.Signal.Span.Name, service)
			}
		}
	}
}

package signalport

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/signal"
)

var signalMutations = []porttest.Mutation{
	{"signals land in the run store under the port's collector", "take the collector from the script input", "red: the stored collector is the one the script named", "TestMetricEventAndSpanAreStoredUnderThePortsCollector"},
	{"the runner time source is reserved", "drop the runner tsSource check", "red: a script span with runner time is stored", "TestRunnerTimeSourceIsDeniedAndHostOrContainerTimePasses"},
	{"oversized signals are refused", "drop the MaxBytes check", "red: a 100 KiB event is stored", "TestSignalOverTheSizeLimitIsPortLimitAndOneWithinItIsStored"},
	{"refusals pass the masker", "build the refusal without mask", "red: the planted value shows in the error", "TestRefusalMessagesPassTheMasker"},
	{"bodies are checked per kind", "skip the metric and span field checks", "red: a metric without a value is stored", "TestIncompleteBodiesAndUnknownMethodsAreBadArgs"},
}

const planted = "PLANTED-cred-7f3a9c"

func stream(t *testing.T) *signal.Stream {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	s, err := signal.Open(filepath.Join(t.TempDir(), "run"), signal.Options{Masker: func() *mask.Masker { return m }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func raw(v string) []json.RawMessage { return []json.RawMessage{json.RawMessage(v)} }

func codeOf(err error) engine.PortCode {
	var pe *engine.PortError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func read(t *testing.T, s *signal.Stream) []contract.Signal {
	t.Helper()
	resp, err := s.Read(signal.ReadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return resp.Signals
}

func TestMetricEventAndSpanAreStoredUnderThePortsCollector(t *testing.T) {
	s := stream(t)
	p := &Port{Collector: "toy-db", Store: s}
	calls := map[string]string{
		"metric": `{"name":"rows","value":3,"attrs":{"table":"orders"},"collector":"evil"}`,
		"event":  `{"body":"order paid","id":"order-7","rev":2}`,
		"span":   `{"traceId":"t1","name":"sync","start":"2026-10-12T07:00:00Z","status":"ok"}`,
	}
	for _, method := range []string{"metric", "event", "span"} {
		if _, err := p.Call(context.Background(), method, raw(calls[method])); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	got := read(t, s)
	if len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	for _, sig := range got {
		if sig.Collector != "toy-db" || sig.TSSource != contract.TimeSourceHost {
			t.Fatalf("%+v", sig)
		}
	}
	if got[0].Metric.Value != 3 || got[1].Event.Body != "order paid" || got[1].Rev != 2 || got[2].Span.TraceID != "t1" {
		t.Fatalf("%+v", got)
	}
}

func TestRunnerTimeSourceIsDeniedAndHostOrContainerTimePasses(t *testing.T) {
	s := stream(t)
	p := &Port{Collector: "toy-db", Store: s}
	_, err := p.Call(context.Background(), "span", raw(`{"traceId":"run-1","name":"run","start":"2026-10-12T07:00:00Z","status":"ok","tsSource":"runner"}`))
	if codeOf(err) != engine.PortDenied || !strings.Contains(err.Error(), "reserved for spans the hub builds") {
		t.Fatalf("%v", err)
	}
	if _, err := p.Call(context.Background(), "event", raw(`{"body":"tail","tsSource":"container"}`)); err != nil {
		t.Fatalf("canary: %v", err)
	}
	if got := read(t, s); len(got) != 1 || got[0].TSSource != contract.TimeSourceContainer {
		t.Fatalf("%+v", got)
	}
}

func TestSignalOverTheSizeLimitIsPortLimitAndOneWithinItIsStored(t *testing.T) {
	s := stream(t)
	p := &Port{Collector: "toy-db", Store: s, MaxBytes: 1024}
	big, _ := json.Marshal(map[string]string{"body": strings.Repeat("x", 100<<10)})
	if _, err := p.Call(context.Background(), "event", []json.RawMessage{big}); codeOf(err) != engine.PortLimit {
		t.Fatalf("%v", err)
	}
	fits, _ := json.Marshal(map[string]string{"body": strings.Repeat("y", 1000)})
	if _, err := p.Call(context.Background(), "event", []json.RawMessage{fits}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, s); len(got) != 1 || len(got[0].Event.Body) != 1000 {
		t.Fatalf("%d", len(got))
	}
}

func TestRefusalMessagesPassTheMasker(t *testing.T) {
	p := &Port{Collector: "toy-db", Store: stream(t), Mask: func(s string) string { return strings.ReplaceAll(s, planted, "****") }}
	_, err := p.Call(context.Background(), "event", raw(`{"body":"x","tsSource":"`+planted+`"}`))
	if err == nil || strings.Contains(err.Error(), planted) || !strings.Contains(err.Error(), "****") {
		t.Fatalf("%v", err)
	}
}

func TestIncompleteBodiesAndUnknownMethodsAreBadArgs(t *testing.T) {
	s := stream(t)
	p := &Port{Collector: "toy-db", Store: s}
	cases := []struct{ method, arg string }{
		{"metric", `{"name":"rows"}`},
		{"metric", `{"value":1}`},
		{"span", `{"name":"x","start":"2026-10-12T07:00:00Z","status":"ok"}`},
		{"span", `{"traceId":"t","name":"x","start":"2026-10-12T07:00:00Z","status":"done"}`},
		{"trace", `{}`},
		{"event", `"not an object"`},
	}
	for _, c := range cases {
		if _, err := p.Call(context.Background(), c.method, raw(c.arg)); codeOf(err) != engine.PortBadArgs {
			t.Errorf("%s %s: %v", c.method, c.arg, err)
		}
	}
	if got := read(t, s); len(got) != 0 {
		t.Fatalf("stored %d", len(got))
	}
	if _, err := p.Call(context.Background(), "metric", raw(`{"name":"rows","value":0}`)); err != nil {
		t.Fatalf("canary: a zero metric was refused: %v", err)
	}
}

func TestSignalMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "signal_test.go", signalMutations)
}

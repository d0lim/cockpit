package headless

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/cli"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/signal"
	"github.com/d0lim/cockpit/internal/tracectx"
)

var signalsMutations = []porttest.Mutation{
	{"the stream and the observers read the refreshed run masker", "drop both r.share() calls after the generation and the runner", "red: the stream freezes the planted container log value, so the rescan discards the bundle and no copy is written", "TestAPassingRunFreezesItsStageAndRunnerSpansAndTheServiceLogsMaskedIntoItsBundle"},
	{"the report appends the stage and runner spans", "pass Recordings without its Stream in report", "red: the frozen copy holds no pipeline, stage, run, flow or step span", "TestAPassingRunFreezesItsStageAndRunnerSpansAndTheServiceLogsMaskedIntoItsBundle"},
	{"the run records each stage from its start to the event that closes it", "drop StartStage from walk", "red: the frozen copy holds no pipeline or stage span", "TestAPassingRunFreezesItsStageAndRunnerSpansAndTheServiceLogsMaskedIntoItsBundle"},
	{"the observers run from the end of the boot stage", "drop the r.observe() call from boot", "red: the frozen copy holds no container log line and no service row", "TestAPassingRunFreezesItsStageAndRunnerSpansAndTheServiceLogsMaskedIntoItsBundle"},
	{"the report freezes the stream into the bundle", "drop bundle.Signals = r.stream from report", "red: the bundle has no signals/stream.jsonl and its signals stage is skipped", "TestAPassingRunFreezesItsStageAndRunnerSpansAndTheServiceLogsMaskedIntoItsBundle"},
	{"the run masker reaches the observers through an atomic pointer", "read r.env.Masker in the stream's masker and in maskShared", "red under -race: a data race between RefreshMasker and an OTLP append", "TestOTLPSpansThatArriveWhileTheRunnerRefreshesTheMaskerJoinTheFrozenRunTraceAndTheDrainIsNamed"},
	{"the run stage waits for the OTLP stream to go quiet", "return an empty note from drainOTLP without settling", "red: the runner line does not name the drain", "TestOTLPSpansThatArriveWhileTheRunnerRefreshesTheMaskerJoinTheFrozenRunTraceAndTheDrainIsNamed"},
}

func TestSignalsMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "signals_test.go", signalsMutations)
}

func timedSteps() []contract.ResultStep {
	steps := passingSteps()
	for i := range steps {
		start := time.Date(2026, 10, 19, 1, 0, 1+i, 0, time.UTC)
		steps[i].StartedAt, steps[i].FinishedAt = store.Timestamp(start), store.Timestamp(start.Add(500*time.Millisecond))
	}
	return steps
}

func frozenStream(t *testing.T, bundle string) []contract.Signal {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(bundle, signal.FrozenPath))
	if err != nil {
		t.Fatal(err)
	}
	var out []contract.Signal
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var sig contract.Signal
		if err := json.Unmarshal([]byte(line), &sig); err != nil {
			t.Fatalf("frozen line %q: %v", line, err)
		}
		out = append(out, sig)
	}
	return out
}

var runTrace = map[string]string{
	"pipeline": "", "stage:prepare": "pipeline", "stage:build": "pipeline", "stage:seal": "pipeline", "stage:boot": "pipeline", "stage:suite": "pipeline",
	"run": "stage:suite", "flow:seed": "run", "flow:health": "run", "flow:orders": "run", "flow:facts": "run",
	"step:seed/seed-users": "flow:seed", "step:health/health-ok": "flow:health", "step:orders/create": "flow:orders", "step:facts/facts-read": "flow:facts",
}

func TestAPassingRunFreezesItsStageAndRunnerSpansAndTheServiceLogsMaskedIntoItsBundle(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	f := newFake(func(f *fakedocker.Fake) {
		f.Steps, f.ContainerSecret, f.RunnerDelay = timedSteps(), planted, 500*time.Millisecond
	})
	code, text := runFake(t, toyProject(t), f, Options{Out: out})
	if code != cli.ExitOK {
		t.Fatalf("exit %d\n%s", code, text)
	}
	summary := readJSON(t, filepath.Join(out, "summary.json"))
	stages, _ := summary["stages"].(map[string]any)
	if !reflect.DeepEqual(stages["signals"], map[string]any{"status": "ok"}) {
		t.Fatalf("signals stage %v", stages["signals"])
	}
	id, _ := summary["runId"].(string)
	spans := map[string]contract.Signal{}
	var logs, services []contract.Signal
	for _, sig := range frozenStream(t, out) {
		switch {
		case sig.Kind == contract.SignalKindSpan && sig.Span.TraceID == id:
			spans[sig.ID] = sig
		case sig.Collector == signal.LogCollector:
			logs = append(logs, sig)
		case sig.Collector == "services":
			services = append(services, sig)
		}
	}
	if len(spans) != len(runTrace) {
		t.Fatalf("%d spans under trace %s, want %d: %v", len(spans), id, len(runTrace), slices.Sorted(maps.Keys(spans)))
	}
	for span, parent := range runTrace {
		sig, ok := spans[span]
		collector := signal.RunnerSource
		if span == signal.PipelineSpanID || strings.HasPrefix(span, signal.StageSpanPrefix) {
			collector = signal.PipelineCollector
		}
		if !ok || sig.Span.ParentID != parent || sig.Collector != collector || sig.Span.Status != contract.SpanStatusOK {
			t.Fatalf("span %s = %+v from %q, want parent %q from %q with status ok", span, sig.Span, sig.Collector, parent, collector)
		}
		if parent != "" && spans[parent].Seq >= sig.Seq {
			t.Errorf("parent %s (seq %d) is not before %s (seq %d)", parent, spans[parent].Seq, span, sig.Seq)
		}
	}
	if len(logs) == 0 || len(services) == 0 {
		t.Fatalf("canary: the observers left %d log lines and %d service rows", len(logs), len(services))
	}
	for _, sig := range logs {
		body := sig.Event.Body
		if !strings.Contains(body, fakedocker.ContainerLogLine) || strings.Contains(body, planted) || !strings.Contains(body, "****") {
			t.Fatalf("container log line %q must hold the line with **** in place of the planted value", body)
		}
	}
}

func otlpSpans(runID string) []fakedocker.OTLPLine {
	var lines []fakedocker.OTLPLine
	for i := range 60 {
		line := fmt.Sprintf(`{"v":1,"signal":{"kind":"span","id":"w3c:%016x","attrs":{"service":"app"},"span":{"traceId":"w3c:%s","name":"app call","start":"2026-10-19T01:00:02.110Z","end":"2026-10-19T01:00:02.120Z","status":"ok"}}}`, i+1, tracectx.TraceID(runID))
		lines = append(lines, fakedocker.OTLPLine{After: time.Duration(5*i) * time.Millisecond, Line: line})
	}
	return lines
}

func TestOTLPSpansThatArriveWhileTheRunnerRefreshesTheMaskerJoinTheFrozenRunTraceAndTheDrainIsNamed(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	f := newFake(func(f *fakedocker.Fake) { f.OTLP = otlpSpans })
	limits := testLimits()
	limits.OTLPQuiet, limits.OTLPDrain = time.Second, 5*time.Second
	code, text := runFakeLimited(context.Background(), t, toyProject(t), f, Options{Out: out}, limits)
	if code != cli.ExitOK || !strings.Contains(text, "otlp drained after") {
		t.Fatalf("exit %d, the runner line must name the drain\n%s", code, text)
	}
	id, _ := readJSON(t, filepath.Join(out, "summary.json"))["runId"].(string)
	joined := 0
	for _, sig := range frozenStream(t, out) {
		if sig.Collector == signal.OTLPCollector && sig.Span != nil && sig.Span.TraceID == id {
			joined++
		}
	}
	if joined < 50 {
		t.Fatalf("%d of 60 service spans joined the frozen run trace", joined)
	}
}

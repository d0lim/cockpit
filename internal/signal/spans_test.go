package signal

import (
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

const runID = "run-20261012-153001-997a37e"

func spanByID(t *testing.T, signals []contract.Signal, id string) contract.Signal {
	t.Helper()
	for _, s := range signals {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no span %q", id)
	return contract.Signal{}
}

func step(flow, id string, status contract.StepStatus, start, end string) contract.ResultStep {
	return contract.ResultStep{Flow: flow, ID: id, Status: status, StartedAt: start, FinishedAt: end}
}

func TestRunnerDyingMidFlowEndsTheRunningStepAtContainerExitAsIncomplete(t *testing.T) {
	res := contract.Result{SchemaVersion: 2, Suite: "smoke", Complete: false, StartedAt: "2026-10-12T07:00:00Z", Steps: []contract.ResultStep{
		step("health", "health-1", contract.StepStatusPassed, "2026-10-12T07:00:01Z", "2026-10-12T07:00:02Z"),
		step("order", "order-1", contract.StepStatusRunning, "2026-10-12T07:00:03Z", ""),
		step("order", "order-2", contract.StepStatusNotRun, "", ""),
	}}
	exit := "2026-10-12T07:00:09Z"
	signals := RunSpans(runID, res, exit)
	running := spanByID(t, signals, "step:order/order-1")
	if running.Span.End != exit || running.Span.Status != contract.SpanStatusIncomplete || running.Span.ParentID != "flow:order" {
		t.Fatalf("%+v", running.Span)
	}
	done := spanByID(t, signals, "step:health/health-1")
	if done.Span.End != "2026-10-12T07:00:02Z" || done.Span.Status != contract.SpanStatusOK {
		t.Fatalf("canary: %+v", done.Span)
	}
	order := spanByID(t, signals, "flow:order")
	run := spanByID(t, signals, RunSpanID)
	if order.Span.Status != contract.SpanStatusIncomplete || order.Span.End != exit || run.Span.Status != contract.SpanStatusIncomplete || run.Span.End != exit {
		t.Fatalf("%+v %+v", order.Span, run.Span)
	}
	if len(signals) != 5 {
		t.Fatalf("spans %d, the step that never started must have none", len(signals))
	}
	for _, s := range signals {
		if s.Collector != RunnerSource || s.TSSource != contract.TimeSourceRunner || s.Span.TraceID != runID {
			t.Fatalf("%+v", s)
		}
	}
}

func TestCompleteRunSpansCarryOkAndErrorUpToTheRunSpan(t *testing.T) {
	res := contract.Result{SchemaVersion: 2, Suite: "smoke", Complete: true, StartedAt: "2026-10-12T07:00:00Z", FinishedAt: "2026-10-12T07:00:05Z", Steps: []contract.ResultStep{
		step("health", "health-1", contract.StepStatusPassed, "2026-10-12T07:00:01Z", "2026-10-12T07:00:02Z"),
		step("order", "order-1", contract.StepStatusFailed, "2026-10-12T07:00:03Z", "2026-10-12T07:00:04Z"),
	}}
	signals := RunSpans(runID, res, "")
	if s := spanByID(t, signals, "flow:health"); s.Span.Status != contract.SpanStatusOK {
		t.Fatalf("%+v", s.Span)
	}
	if s := spanByID(t, signals, "flow:order"); s.Span.Status != contract.SpanStatusError {
		t.Fatalf("%+v", s.Span)
	}
	if s := spanByID(t, signals, RunSpanID); s.Span.Status != contract.SpanStatusError || s.Span.End != res.FinishedAt || s.Span.Name != "smoke" {
		t.Fatalf("%+v", s.Span)
	}
}

func TestExecIsANewTraceLinkedToTheRunSpan(t *testing.T) {
	res := contract.ExecResult{Target: "order", Result: contract.Result{SchemaVersion: 2, Suite: "smoke", Complete: true, StartedAt: "2026-10-12T08:00:00Z", FinishedAt: "2026-10-12T08:00:02Z", Steps: []contract.ResultStep{
		step("order", "order-1", contract.StepStatusPassed, "2026-10-12T08:00:01Z", "2026-10-12T08:00:02Z"),
	}}}
	signals := ExecSpans(runID, "20261012-080000-order", res, "")
	root := spanByID(t, signals, ExecSpanID)
	if root.Span.TraceID == runID || root.Span.Name != "order" || len(root.Span.Links) != 1 || root.Span.Links[0] != (contract.SpanLink{TraceID: runID, SpanID: RunSpanID}) {
		t.Fatalf("%+v", root.Span)
	}
	if s := spanByID(t, signals, "flow:order"); s.Span.ParentID != ExecSpanID || s.Span.TraceID != root.Span.TraceID {
		t.Fatalf("%+v", s.Span)
	}
}

func TestRunnerSpansAppendToTheStreamAsSpanSignals(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	res := contract.Result{SchemaVersion: 2, Suite: "smoke", Complete: true, StartedAt: "2026-10-12T07:00:00Z", FinishedAt: "2026-10-12T07:00:02Z", Steps: []contract.ResultStep{
		step("health", "health-1", contract.StepStatusPassed, "2026-10-12T07:00:01Z", "2026-10-12T07:00:02Z"),
	}}
	for _, sig := range RunSpans(runID, res, "") {
		mustAppend(t, s, sig)
	}
	if got := readAll(t, s, 0); len(got.Signals) != 3 || got.Signals[0].Kind != contract.SignalKindSpan {
		t.Fatalf("%+v", got.Signals)
	}
}

func TestStepRefsBecomeRefAttributesAndCallsBecomeSpansUnderTheStep(t *testing.T) {
	ok, bad := 201, 409
	st := step("order", "place", contract.StepStatusPassed, "2026-10-12T07:00:01Z", "2026-10-12T07:00:02Z")
	st.Refs = []contract.StepRef{{Name: "order", Value: "ord-1"}, {Name: "order", Value: "ord-2"}}
	st.Calls = []contract.StepCall{
		{Method: "POST", Host: "app", Path: "/orders", Status: &ok, StartedAt: "2026-10-12T07:00:01.100Z", DurationMs: 250},
		{Method: "POST", Host: "app", Path: "/orders", Status: &bad, StartedAt: "2026-10-12T07:00:01.400Z", DurationMs: 30},
		{Method: "GET", Host: "app", Path: "/orders/ord-1", StartedAt: "2026-10-12T07:00:01.500Z"},
	}
	res := contract.Result{SchemaVersion: 2, Suite: "smoke", Complete: true, StartedAt: "2026-10-12T07:00:00Z", FinishedAt: "2026-10-12T07:00:05Z", Steps: []contract.ResultStep{st}}
	signals := RunSpans(runID, res, "")
	stepSpan := spanByID(t, signals, "step:order/place")
	if stepSpan.Attrs["ref.order"] != "ord-1" || stepSpan.Attrs["ref.order.2"] != "ord-2" {
		t.Fatalf("step attrs %v", stepSpan.Attrs)
	}
	first := spanByID(t, signals, "call:order/place/1")
	if first.Span.ParentID != "step:order/place" || first.Span.Name != "POST /orders" || first.Span.Status != contract.SpanStatusOK || first.Span.End != "2026-10-12T07:00:01.350Z" || first.Attrs["status"] != 201 {
		t.Fatalf("first call %+v %v", first.Span, first.Attrs)
	}
	if s := spanByID(t, signals, "call:order/place/2"); s.Span.Status != contract.SpanStatusError {
		t.Fatalf("a 409 call is %s", s.Span.Status)
	}
	third := spanByID(t, signals, "call:order/place/3")
	if _, has := third.Attrs["status"]; has || third.Span.Status != contract.SpanStatusError || third.Span.End != "2026-10-12T07:00:01.500Z" {
		t.Fatalf("a call without a response %+v %v", third.Span, third.Attrs)
	}
	if len(signals) != 6 {
		t.Fatalf("%d spans", len(signals))
	}
}

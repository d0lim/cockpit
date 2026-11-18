package wiretest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/wire"
)

const (
	PublicToken = "tok-1f2e3d4c"
	SpanFirst   = "00f067aa0ba902b7"
	SpanFailed  = "00f067aa0ba902b8"
	Runner      = "cockpit-runner"
)

var T0 = time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC)

func at(d time.Duration) time.Time { return T0.Add(d) }

func stamp(d time.Duration) string { return at(d).Format(time.RFC3339Nano) }

type Take struct {
	Name         string
	Calls        []wire.Call
	Result       *contract.Result
	Routes       []contract.WireRoute
	PublicFields []contract.WirePublicField
	Known        []string
}

func Store(t testing.TB, runDir string, in Take) wire.Status {
	t.Helper()
	if in.Name == "" {
		in.Name = wire.SuiteTake
	}
	status := wire.Take(context.Background(), wire.TakeInput{
		Store: wire.OpenStore(runDir), Name: in.Name, Result: in.Result, Routes: in.Routes, PublicFields: in.PublicFields,
		Record: func() (*mask.Masker, error) { return recordMasker(in) },
		Stream: func(_ context.Context, out io.Writer) error { return writeLines(out, in.Calls) },
	})
	if status.State != wire.StateTaken {
		t.Fatalf("the take was not stored: %+v", status)
	}
	return status
}

func recordMasker(in Take) (*mask.Masker, error) {
	m, err := mask.New(mask.Options{Mode: mask.Runner, Known: in.Known})
	if err != nil {
		return nil, err
	}
	fields := make([]mask.PublicField, 0, len(in.PublicFields))
	for _, f := range in.PublicFields {
		fields = append(fields, mask.PublicField(f))
	}
	return m.WithPublicFields(fields)
}

func writeLines(out io.Writer, calls []wire.Call) error {
	for i := range calls {
		if err := writeLine(out, wire.Line{Call: &calls[i]}); err != nil {
			return err
		}
	}
	return writeLine(out, wire.Line{End: &wire.End{Calls: len(calls)}})
}

func writeLine(out io.Writer, line wire.Line) error {
	raw, err := json.Marshal(line)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(raw))
	return err
}

func Routes() []contract.WireRoute {
	return []contract.WireRoute{
		{Name: "orders", Service: "orders", Port: 8080},
		{Name: "ledger", Service: "ledger", Port: 8081},
		{Name: "mail", Service: "mail", Port: 2525},
	}
}

func PublicFields() []contract.WirePublicField {
	return []contract.WirePublicField{{Key: "token", Value: "tok-[0-9a-f]{8}"}}
}

func Result() *contract.Result {
	return &contract.Result{Steps: []contract.ResultStep{
		{Flow: "alpha", ID: "a1", Status: contract.StepStatusPassed, StartedAt: stamp(0), FinishedAt: stamp(10 * time.Second), Calls: []contract.StepCall{{SpanID: SpanFirst}}},
		{Flow: "alpha", ID: "a2", Status: contract.StepStatusFailed, StartedAt: stamp(10 * time.Second), FinishedAt: stamp(20 * time.Second), Calls: []contract.StepCall{{SpanID: SpanFailed}}},
		{Flow: "beta", ID: "b1", Status: contract.StepStatusPassed, StartedAt: stamp(30 * time.Second), FinishedAt: stamp(31 * time.Second)},
	}}
}

func call(seq int64, caller, target string, port int, method, path string, from, to time.Duration, status int) wire.Call {
	return wire.Call{
		Seq: seq, Caller: caller, Target: target, Port: port, Method: method, Path: path,
		Started: at(from), Finished: at(to), Status: status, Outcome: wire.OutcomeAnswered,
	}
}

func Calls(secret string) []wire.Call {
	first := call(1, Runner, "orders", 8080, "POST", "/orders", time.Second, 5*time.Second, 201)
	first.Span = SpanFirst
	first.Response = wire.Message{Size: 12, Body: wire.Body{Kind: wire.BodyJSON, JSON: map[string]any{"id": "ord-1"}}}
	nested := call(2, "orders", "ledger", 8081, "GET", "/v1/entries", 2*time.Second, 3*time.Second, 200)
	nested.Query = wire.Body{Kind: wire.BodyForm, Form: map[string][]string{"limit": {"5"}}}
	nested.Response = wire.Message{Size: 2, Body: wire.Body{Kind: wire.BodyText, Text: "ok"}}
	timed := call(3, "worker", "ledger", 8081, "POST", "/v1/ping", 4*time.Second, 4500*time.Millisecond, 204)

	failed := call(4, Runner, "orders", 8080, "POST", "/orders/"+secret, 11*time.Second, 15*time.Second, 500)
	failed.Span = SpanFailed
	failed.Query = wire.Body{Kind: wire.BodyForm, Form: map[string][]string{"ref": {secret}}}
	failed.Request = wire.Message{
		Headers: map[string][]string{"content-type": {"application/json"}, "x-request-id": {secret}, "authorization": {"Basic abc"}},
		Size:    64, Body: wire.Body{Kind: wire.BodyJSON, JSON: map[string]any{"note": secret, "token": PublicToken, "password": "hunter2"}},
	}
	failed.Response = wire.Message{Size: 32, Body: wire.Body{Kind: wire.BodyText, Text: "failed for " + secret}}
	inner := call(5, "orders", "ledger", 8081, "POST", "/v1/entries", 12*time.Second, 13*time.Second, 200)
	inner.Request = wire.Message{Size: 20, Body: wire.Body{Kind: wire.BodyForm, Form: map[string][]string{"amount": {"7"}, "memo": {secret}}}}
	inner.Response = wire.Message{Size: 90, Body: wire.Body{Kind: wire.BodyJWT, JWT: &wire.JWT{Header: map[string]any{"alg": "none"}, Claims: map[string]any{"sub": "user-1", "note": secret}}}}
	deep := call(6, "ledger", "mail", 2525, "POST", "/send", 12100*time.Millisecond, 12500*time.Millisecond, 202)
	deep.Response = wire.Message{Size: 2202009, Body: wire.Body{Kind: wire.BodyOmitted, Omitted: wire.OmitTooLarge}}
	hidden := call(7, "worker", "ledger", 8081, "GET", "/v1/hidden", 16*time.Second, 17*time.Second, 200)
	hidden.Withheld = true
	late := call(8, "worker", "orders", 8080, "GET", "/late", 25*time.Second, 26*time.Second, 200)
	return []wire.Call{first, nested, timed, failed, inner, deep, hidden, late}
}

func Seed(t testing.TB, runDir, secret string) wire.Status {
	t.Helper()
	return Store(t, runDir, Take{Calls: Calls(secret), Result: Result(), Routes: Routes(), PublicFields: PublicFields()})
}

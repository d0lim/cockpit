package sqlport

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/collect/collecttest"
	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var paramMutations = []porttest.Mutation{
	{"params reach the driver as bound values and never as statement text", "splice the params into the statement in run and pass no args", "red: the recorded statement holds the injected text and no args are bound", "TestParamsReachTheDriverAsBoundValuesAndTheStatementTextStaysAsWritten"},
	{"only strings, numbers, booleans and null are params", "return nil from the default case of checkParams", "red: objects and arrays reach the driver", "TestObjectArrayAndUnreadableParamsArePortBadArgsBeforeProvisioningAndScalarsPass"},
	{"param refusals do not echo the value", "append the value to the refusal", "red: the planted value shows in the error", "TestObjectArrayAndUnreadableParamsArePortBadArgsBeforeProvisioningAndScalarsPass"},
	{"param count and size are capped", "drop the count and size checks in checkParams", "red: 101 params and 64 KiB plus one byte reach the driver", "TestParamsOverTheCountOrSizeLimitArePortLimitAndParamsAtTheLimitPass"},
	{"undefined params are refused before the call leaves the script host", "drop the undefinedParam check in the collect module", "red: undefined reaches the port as null and the query runs", "TestCollectorParamsBindThroughTheScriptHostAndAnUndefinedParamIsPortBadArgs"},
}

func TestParamsMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "params_test.go", paramMutations)
}

func rawArgs(t *testing.T, values ...any) []json.RawMessage {
	t.Helper()
	out := make([]json.RawMessage, len(values))
	for i, v := range values {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = raw
	}
	return out
}

func TestParamsReachTheDriverAsBoundValuesAndTheStatementTextStaysAsWritten(t *testing.T) {
	state := itemRows(1)
	p, _ := fakePort(t, state, Options{})
	const stmt = "SELECT id, name FROM app.items WHERE name = ? AND id = ? AND score > ? AND live = ? AND note IS ?"
	injected := "x'; DROP TABLE app.items; --"
	opts := map[string]any{"params": []any{injected, 7, 1.5, true, nil}}
	if _, err := p.Call(context.Background(), "query", rawArgs(t, "main", stmt, opts)); err != nil {
		t.Fatal(err)
	}
	if len(state.statements) != 1 || state.statements[0] != stmt {
		t.Fatalf("statements %q", state.statements)
	}
	want := []any{injected, int64(7), 1.5, true, nil}
	if len(state.args) != 1 || !slices.Equal(state.args[0], want) {
		t.Fatalf("bound %#v, want %#v", state.args, want)
	}
	if _, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1"}); err != nil || len(state.args[1]) != 0 {
		t.Fatalf("a query without params bound %v: %v", state.args, err)
	}
}

func TestObjectArrayAndUnreadableParamsArePortBadArgsBeforeProvisioningAndScalarsPass(t *testing.T) {
	p, fb := fakePort(t, itemRows(1), Options{})
	bad := []struct {
		opts string
		want string
	}{
		{`{"params":[{"token":"` + planted + `"}]}`, "params[0] is an object"},
		{`{"params":["ok",["` + planted + `"]]}`, "params[1] is an array"},
		{`{"params":[1e400]}`, "params[0] is not a plain value"},
		{`{"params":"` + planted + `"}`, "options does not match the expected shape"},
		{`{"params":{"0":1}}`, "options does not match the expected shape"},
	}
	refused := 0
	for _, c := range bad {
		args := []json.RawMessage{json.RawMessage(`"main"`), json.RawMessage(`"SELECT 1"`), json.RawMessage(c.opts)}
		_, err := p.Call(context.Background(), "query", args)
		if codeOf(err) != engine.PortBadArgs || !strings.Contains(err.Error(), c.want) || strings.Contains(err.Error(), planted) {
			t.Errorf("%s: %v", c.opts, err)
		}
		refused++
	}
	if refused != len(bad) || fb.provisions.Load() != 0 {
		t.Fatalf("refused %d of %d, provisioned %d times", refused, len(bad), fb.provisions.Load())
	}
	ok := []string{`{"params":["x",1,-2.5,true,false,null]}`, `{"params":[]}`, `{"params":null}`, `{}`}
	passed := 0
	for _, opts := range ok {
		args := []json.RawMessage{json.RawMessage(`"main"`), json.RawMessage(`"SELECT 1"`), json.RawMessage(opts)}
		if _, err := p.Call(context.Background(), "query", args); err != nil {
			t.Errorf("%s: %v", opts, err)
		}
		passed++
	}
	if passed != 4 {
		t.Fatalf("passed %d", passed)
	}
	if err := checkParams([]any{struct{}{}}, ports.Identity); codeOf(err) != engine.PortBadArgs {
		t.Fatalf("a Go struct param: %v", err)
	}
}

func TestParamsOverTheCountOrSizeLimitArePortLimitAndParamsAtTheLimitPass(t *testing.T) {
	state := itemRows(1)
	p, _ := fakePort(t, state, Options{})
	count := func(n int) []any {
		out := make([]any, n)
		for i := range out {
			out[i] = int64(i)
		}
		return out
	}
	over := [][]any{count(maxParams + 1), {strings.Repeat("a", maxParamBytes+1)}, {strings.Repeat("a", maxParamBytes-scalarSize+1), nil}}
	for i, params := range over {
		_, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1", Params: params})
		if codeOf(err) != engine.PortLimit {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if len(state.statements) != 0 {
		t.Fatalf("%d queries ran over the limit", len(state.statements))
	}
	at := [][]any{count(maxParams), {strings.Repeat("a", maxParamBytes)}, {strings.Repeat("a", maxParamBytes-scalarSize), nil}}
	for i, params := range at {
		if _, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1", Params: params}); err != nil {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if len(state.statements) != len(at) || len(state.args[0]) != maxParams {
		t.Fatalf("%d queries ran at the limit", len(state.statements))
	}
}

func TestCollectorParamsBindThroughTheScriptHostAndAnUndefinedParamIsPortBadArgs(t *testing.T) {
	state := itemRows(1)
	p, _ := fakePort(t, state, Options{})
	code := collecttest.Bundle(t, "toy-db", `
		const r = await ctx.sql.query("main", "SELECT id, name FROM app.items WHERE name = ?", { params: ["item", 2, null] });
		const codes = [];
		for (const params of [[undefined], [1, , 3], [{ id: 1 }]]) {
			try { await ctx.sql.query("main", "SELECT 1", { params }); codes.push("none"); } catch (e) { codes.push(e.code + " " + e.message); }
		}
		ctx.signal.event({ attrs: { rows: JSON.stringify(r.rows), codes: codes.join("|") } });
	`)
	signal := &collecttest.Recorder{}
	out, err := worker.Run(context.Background(), collecttest.Spec(code, []string{ports.PortSQL, ports.PortSignal}, map[string]ports.Handler{ports.PortSQL: p, ports.PortSignal: signal}))
	if err != nil || out.Status != worker.StatusOK {
		t.Fatalf("%+v %v", out, err)
	}
	calls := signal.Calls()
	if len(calls) != 1 {
		t.Fatalf("%d signal calls", len(calls))
	}
	var sent struct {
		Attrs map[string]string `json:"attrs"`
	}
	if err := json.Unmarshal(calls[0].Args[0], &sent); err != nil {
		t.Fatal(err)
	}
	codes := strings.Split(sent.Attrs["codes"], "|")
	if sent.Attrs["rows"] != `[[1,"item"]]` || len(codes) != 3 {
		t.Fatalf("%+v", sent.Attrs)
	}
	for i, want := range []string{"PORT_BAD_ARGS params[0] is undefined", "PORT_BAD_ARGS params[1] is undefined", "PORT_BAD_ARGS params[0] is an object"} {
		if !strings.HasPrefix(codes[i], want) {
			t.Errorf("case %d: %q, want %q", i, codes[i], want)
		}
	}
	if len(state.args) != 1 || !slices.Equal(state.args[0], []any{"item", int64(2), nil}) {
		t.Fatalf("bound %#v", state.args)
	}
}

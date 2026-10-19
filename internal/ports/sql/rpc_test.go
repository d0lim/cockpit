package sqlport

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/collect/collecttest"
	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var rpcMutations = []porttest.Mutation{
	{"collector scripts reach the sql port through the script host", "return nil, nil from Call", "red: the script sees no rows and fails", "TestCollectorScriptQueriesTheSQLPortByNameAndAnUndeclaredNameBecomesAnUnreadRecord"},
	{"query options reach the port", "drop the options argument in Call", "red: two rows come back with truncated false", "TestCollectorScriptQueriesTheSQLPortByNameAndAnUndeclaredNameBecomesAnUnreadRecord"},
	{"unknown methods and missing arguments are refused", "dispatch every method to Query", "red: sql.exec or query without a statement does not fail", "TestUnknownSQLMethodsAndMissingArgumentsArePortBadArgs"},
}

func TestCollectorScriptQueriesTheSQLPortByNameAndAnUndeclaredNameBecomesAnUnreadRecord(t *testing.T) {
	p, _ := fakePort(t, itemRows(2), Options{})
	code := collecttest.Bundle(t, "toy-db", `
		const r = await ctx.sql.query("main", "SELECT id, name FROM app.items", { maxRows: 1 });
		let denied = "";
		try { await ctx.sql.query("other", "SELECT 1"); } catch (e) { denied = e.code; }
		ctx.signal.event({ attrs: { rows: JSON.stringify(r.rows), truncated: r.truncated, denied } });
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
		Attrs map[string]any `json:"attrs"`
	}
	if err := json.Unmarshal(calls[0].Args[0], &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Attrs["rows"] != `[[1,"item"]]` || sent.Attrs["truncated"] != true || sent.Attrs["denied"] != string(engine.PortDenied) {
		t.Fatalf("%+v", sent.Attrs)
	}
	if len(out.Unread) != 1 || out.Unread[0].Source != "port:sql" || !strings.Contains(out.Unread[0].Reason, `"other" is not declared`) {
		t.Fatalf("%+v", out.Unread)
	}
}

func TestUnknownSQLMethodsAndMissingArgumentsArePortBadArgs(t *testing.T) {
	p, fb := fakePort(t, itemRows(1), Options{})
	bad := []struct {
		method string
		args   []json.RawMessage
	}{
		{"exec", []json.RawMessage{json.RawMessage(`"main"`), json.RawMessage(`"SELECT 1"`)}},
		{"query", []json.RawMessage{json.RawMessage(`"main"`)}},
		{"query", nil},
	}
	for _, c := range bad {
		if _, err := p.Call(context.Background(), c.method, c.args); codeOf(err) != engine.PortBadArgs {
			t.Errorf("%s %s: %v", c.method, c.args, err)
		}
	}
	if len(bad) != 3 || fb.provisions.Load() != 0 {
		t.Fatalf("a bad call provisioned (%d cases)", len(bad))
	}
	if _, err := p.Call(context.Background(), "query", []json.RawMessage{json.RawMessage(`"main"`), json.RawMessage(`"SELECT 1"`)}); err != nil || fb.provisions.Load() != 1 {
		t.Fatalf("canary: %v", err)
	}
}

func TestRPCMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "rpc_test.go", rpcMutations)
}

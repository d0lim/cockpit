package dockerport

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/collect/collecttest"
	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var rpcMutations = []porttest.Mutation{
	{"collector scripts reach the docker port through the script host", "return nil, nil from Call", "red: the script sees no services and fails", "TestCollectorScriptReadsItsRunThroughTheDockerPortAndADenialBecomesAnUnreadRecord"},
	{"a docker denial from a script becomes an unread record", "map PORT_DENIED to PORT_UNAVAILABLE in resolve", "red: Outcome.Unread is empty", "TestCollectorScriptReadsItsRunThroughTheDockerPortAndADenialBecomesAnUnreadRecord"},
	{"output reaching the script passes the masker", "drop mask in clip", "red: the planted value reaches the signal call", "TestCollectorScriptReadsItsRunThroughTheDockerPortAndADenialBecomesAnUnreadRecord"},
	{"unknown methods and missing arguments are refused", "dispatch every method to PS", "red: docker.rm and logs without a service do not fail", "TestUnknownDockerMethodsAndMissingArgumentsArePortBadArgs"},
}

func TestCollectorScriptReadsItsRunThroughTheDockerPortAndADenialBecomesAnUnreadRecord(t *testing.T) {
	f := &fakeDocker{handle: func(_ context.Context, cmd compose.Command) (compose.Result, error) {
		switch cmd.Args[0] {
		case "logs":
			write(cmd, "boot token="+planted+"\n", "")
		case "exec":
			write(cmd, "value", "")
		}
		return compose.Result{}, nil
	}}
	p, _ := newPort(t, f, Options{Allow: [][]string{{"valkey-cli", "GET"}}})
	code := collecttest.Bundle(t, "toy-db", `
		const ps = await ctx.docker.ps();
		if (ps.length !== 3 || ps[0].service !== "app") throw new Error("ps: " + JSON.stringify(ps));
		const logs = await ctx.docker.logs("app", { maxBytes: 1000 });
		let denied = "";
		try { await ctx.docker.logs("intruder"); } catch (e) { denied = e.code; }
		const exec = await ctx.docker.exec("app", ["valkey-cli", "GET", "k"], { timeoutMs: 1000 });
		ctx.signal.event({ attrs: { logs: logs.text, denied, exec: exec.stdout } });
	`)
	signal := &collecttest.Recorder{}
	out, err := worker.Run(context.Background(), collecttest.Spec(code, []string{ports.PortDocker, ports.PortSignal}, map[string]ports.Handler{ports.PortDocker: p, ports.PortSignal: signal}))
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
	if sent.Attrs["logs"] != "boot token=****\n" || strings.Contains(string(calls[0].Args[0]), planted) || sent.Attrs["denied"] != "PORT_DENIED" || sent.Attrs["exec"] != "value" {
		t.Fatalf("%+v", sent.Attrs)
	}
	if len(out.Unread) != 1 || out.Unread[0].Source != "port:docker" || !strings.Contains(out.Unread[0].Reason, `"intruder" has no container in this run`) {
		t.Fatalf("%+v", out.Unread)
	}
}

func TestUnknownDockerMethodsAndMissingArgumentsArePortBadArgs(t *testing.T) {
	f := &fakeDocker{handle: func(_ context.Context, cmd compose.Command) (compose.Result, error) {
		write(cmd, `{"PIDs":"1"}`, "")
		return compose.Result{}, nil
	}}
	p, _ := newPort(t, f, Options{})
	bad := []struct {
		method string
		args   []json.RawMessage
	}{
		{"rm", nil},
		{"logs", nil},
		{"file", []json.RawMessage{json.RawMessage(`"app"`)}},
		{"exec", []json.RawMessage{json.RawMessage(`"app"`), json.RawMessage(`"valkey-cli GET k"`)}},
	}
	for _, c := range bad {
		if _, err := p.Call(context.Background(), c.method, c.args); codeOf(err) != "PORT_BAD_ARGS" {
			t.Errorf("%s: %v", c.method, err)
		}
	}
	if len(bad) != 4 || len(f.commands("logs"))+len(f.commands("cp"))+len(f.commands("exec")) != 0 {
		t.Fatalf("docker ran for a bad call: %v", f.calls)
	}
	res, err := p.Call(context.Background(), "stats", []json.RawMessage{json.RawMessage(`"app"`)})
	if stats, ok := res.(map[string]string); err != nil || !ok || stats["PIDs"] != "1" || len(f.commands("stats")) != 1 {
		t.Fatalf("canary: a valid call reaches docker: %v %v", res, err)
	}
}

func TestRPCMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "rpc_test.go", rpcMutations)
}

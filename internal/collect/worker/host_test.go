package worker_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect/collecttest"
	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var hostMutations = []porttest.Mutation{
	{"calls to ports outside the allowlist are denied", "skip the Allowed check in serve", "red: the docker recorder receives the call", "TestCallToAPortOutsideTheAllowlistIsDeniedAndRecordedAsAnUnreadSource"},
	{"denials become unread-record sources", "skip the unread append in reply", "red: Outcome.Unread is empty", "TestCallToAPortOutsideTheAllowlistIsDeniedAndRecordedAsAnUnreadSource"},
	{"allowed calls return their result to the child", "reply with an empty result", "red: the script sees no rows and fails", "TestAllowedPortCallReturnsItsResultToTheScript"},
	{"the call budget kills the child", "skip the MaxCalls check", "red: the outcome is ok after 50 calls with a budget of 10", "TestCallsOverTheBudgetKillTheChild"},
	{"frames over the size limit kill the child", "read frames without the size limit", "red: the outcome is not killed", "TestFrameOverTheSizeLimitKillsTheChild"},
	{"the child environment holds only PATH", "pass os.Environ() to the child", "red: the planted variable is in the child environment", "TestChildEnvironmentHoldsOnlyPath"},
	{"in-flight calls are capped", "skip the MaxInFlight check", "red: all 8 concurrent calls reach the port", "TestConcurrentCallsOverTheInFlightCapAreRefusedWithPortLimit"},
}

func run(t *testing.T, spec worker.Spec) worker.Outcome {
	t.Helper()
	out, err := worker.Run(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCallToAPortOutsideTheAllowlistIsDeniedAndRecordedAsAnUnreadSource(t *testing.T) {
	docker := &collecttest.Recorder{Result: []any{}}
	code := collecttest.Bundle(t, "toy-db", `
		let denied = "";
		try { await ctx.docker.ps(); } catch (e) { denied = e.code + " " + e.message; }
		if (!denied.startsWith("PORT_DENIED")) throw new Error("not denied: " + denied);
		await ctx.state.get("cursor");
	`)
	state := &collecttest.Recorder{}
	out := run(t, collecttest.Spec(code, []string{ports.PortState}, map[string]ports.Handler{ports.PortDocker: docker, ports.PortState: state}))
	if out.Status != worker.StatusOK {
		t.Fatalf("%+v", out)
	}
	if len(docker.Calls()) != 0 {
		t.Fatalf("docker received %v", docker.Calls())
	}
	if len(out.Unread) != 1 || out.Unread[0].Source != "port:docker" || !strings.Contains(out.Unread[0].Reason, `port "docker" is not allowed`) {
		t.Fatalf("%+v", out.Unread)
	}
	if len(state.Calls()) != 1 {
		t.Fatalf("canary: the allowed port got %d calls", len(state.Calls()))
	}
}

func TestAllowedPortCallReturnsItsResultToTheScript(t *testing.T) {
	sql := &collecttest.Recorder{Result: map[string]any{"columns": []string{"n"}, "rows": [][]any{{7}}, "truncated": false}}
	code := collecttest.Bundle(t, "toy-db", `
		const r = await ctx.sql.query("app", "select 7 as n");
		if (r.rows[0][0] !== 7 || r.truncated !== false) throw new Error("bad result " + JSON.stringify(r));
	`)
	out := run(t, collecttest.Spec(code, []string{ports.PortSQL}, map[string]ports.Handler{ports.PortSQL: sql}))
	calls := sql.Calls()
	if out.Status != worker.StatusOK || len(calls) != 1 || calls[0].Method != "query" || string(calls[0].Args[1]) != `"select 7 as n"` {
		t.Fatalf("%+v %+v", out, calls)
	}
}

func TestCallsOverTheBudgetKillTheChild(t *testing.T) {
	state := &collecttest.Recorder{}
	code := collecttest.Bundle(t, "toy-db", `for (let i = 0; i < 50; i++) await ctx.state.get("k");`)
	spec := collecttest.Spec(code, []string{ports.PortState}, map[string]ports.Handler{ports.PortState: state})
	spec.Limits.MaxCalls = 10
	if out := run(t, spec); out.Status != worker.StatusKilled || out.Kill != worker.KillCalls {
		t.Fatalf("%+v", out)
	}
	spec.Limits.MaxCalls = 100
	if out := run(t, spec); out.Status != worker.StatusOK || out.Calls != 50 {
		t.Fatalf("canary: %+v", out)
	}
}

func TestFrameOverTheSizeLimitKillsTheChild(t *testing.T) {
	state := &collecttest.Recorder{}
	code := collecttest.Bundle(t, "toy-db", `await ctx.state.set("k", "x".repeat(200000));`)
	spec := collecttest.Spec(code, []string{ports.PortState}, map[string]ports.Handler{ports.PortState: state})
	spec.Limits.MaxFrame = 64 << 10
	if out := run(t, spec); out.Status != worker.StatusKilled || out.Kill != worker.KillFrame {
		t.Fatalf("%+v", out)
	}
	spec.Limits.MaxFrame = 1 << 20
	if out := run(t, spec); out.Status != worker.StatusOK {
		t.Fatalf("canary: %+v", out)
	}
}

func TestChildEnvironmentHoldsOnlyPath(t *testing.T) {
	env := map[string]string{"PATH": "/usr/bin:/bin", "COCKPIT_PLANTED_TOKEN": "PLANTED-hub-env-7f3a9c", "HOME": "/Users/someone"}
	got := worker.ChildEnv(func(k string) string { return env[k] })
	if !slices.Equal(got, []string{"PATH=/usr/bin:/bin"}) {
		t.Fatalf("%v", got)
	}
}

func TestConcurrentCallsOverTheInFlightCapAreRefusedWithPortLimit(t *testing.T) {
	state := &collecttest.Recorder{Delay: 300 * time.Millisecond}
	code := collecttest.Bundle(t, "toy-db", `
		const results = await Promise.allSettled(Array.from({ length: 8 }, () => ctx.state.get("k")));
		const limited = results.filter((r) => r.status === "rejected" && r.reason.code === "PORT_LIMIT").length;
		if (limited !== 6) throw new Error("limited " + limited);
	`)
	spec := collecttest.Spec(code, []string{ports.PortState}, map[string]ports.Handler{ports.PortState: state})
	spec.Limits.MaxInFlight = 2
	if out := run(t, spec); out.Status != worker.StatusOK || len(state.Calls()) != 2 {
		t.Fatalf("%+v calls %d", out, len(state.Calls()))
	}
}

func TestHostMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "host_test.go", hostMutations)
}

package runner

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var wireDialMutations = []porttest.Mutation{
	{"a route's origin is dialed through the recorder", "pass no Reroute to the http client", "red: the dial goes to fake.local", "TestAnAllowedOriginWithARouteDialsTheRecorderAndKeepsItsURLAndHost"},
	{"host names compare without case", "compare the dialed address as written", "red: the mixed-case origin is dialed directly", "TestAnAllowedOriginWithARouteDialsTheRecorderAndKeepsItsURLAndHost"},
	{"an origin without a route keeps its address", "send every dial to the recorder", "red: other.local is dialed through the recorder", "TestAnOriginWithoutARouteDialsItsOwnAddressInARecordingRun"},
	{"a route does not widen the allowlist", "add the routed upstream to the allowed origins", "red: the unlisted origin is dialed", "TestARouteDoesNotAllowAnOriginThatEnvJSONDoesNotList"},
	{"the placeholder expands only for a route in a recording run", "always expand to the default", "red: the value keeps the direct address while recording", "TestThePlaceholderExpandsToTheProxyOnlyForARouteInARecordingRun"},
	{"only the placeholder is expanded", "expand every ${...}", "red: another variable's placeholder is rewritten", "TestThePlaceholderExpandsToTheProxyOnlyForARouteInARecordingRun"},
	{"an expanded value joins the allowlist", "expand after building the allowlist", "red: the request to the proxy origin is refused", "TestAnExpandedEnvValueIsAnAllowedOriginAndItsDefaultRunsWithoutRecording"},
	{"a malformed route list stops the runner", "ignore the parse error in Main", "red: the runner exits 0", "TestAMalformedRouteListExitsTwoBeforeAnyRequest"},
	{"the hub's route format is what the runner reads", "change the separator in WireRunnerRoutes", "red: the rendered routes do not parse", "TestTheRoutesTheHarnessRendersAreTheRoutesTheRunnerReads"},
}

func TestWireDialMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "wiredial_test.go", wireDialMutations)
}

const wireProbeSuite = `import { type Ctx, flow, stage, step, suite } from "cockpit:flow";

export default suite({
	name: "fake",
	stages: [stage({ id: "use", title: "Use" })],
	flows: [flow({ id: "alpha", title: "alpha", stage: "use", once: false, requires: [], steps: [
		step({ id: "call", title: "call", kind: "request", run: async (ctx: Ctx) => {
			await ctx.http.send("GET", String(ctx.env.target) + "/probe");
		} }),
	] })],
});
`

func wireWorld(t *testing.T, env string) *world {
	t.Helper()
	w := newWorld(t, core, nil)
	if err := os.WriteFile(filepath.Join(w.scripts, "suites", "fake.ts"), []byte(wireProbeSuite), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.scripts, "env.json"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	return w
}

func recording(routes string) map[string]string {
	return map[string]string{contract.WireRoutesVariable: routes}
}

func TestAnAllowedOriginWithARouteDialsTheRecorderAndKeepsItsURLAndHost(t *testing.T) {
	w := wireWorld(t, `{"target":"http://Orders.Local:8080"}`)
	if got := w.run(recording("orders=orders.local:8080=cockpit-wire:41001")); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	if !slices.Equal(w.dials, []string{"cockpit-wire:41001"}) || !slices.Equal(w.calls, []string{"/probe"}) {
		t.Fatalf("dials %v calls %v", w.dials, w.calls)
	}
	if !slices.Equal(w.hosts, []string{"Orders.Local:8080"}) {
		t.Fatalf("hosts %v", w.hosts)
	}
	var result contract.Result
	w.read("result.json", &result)
	if call := result.Steps[0].Calls[0]; call.Host != "Orders.Local" || call.Path != "/probe" {
		t.Fatalf("recorded call %+v", call)
	}
}

func TestAnOriginWithoutARouteDialsItsOwnAddressInARecordingRun(t *testing.T) {
	w := wireWorld(t, `{"target":"http://other.local:9000"}`)
	if got := w.run(recording("orders=orders.local:8080=cockpit-wire:41001")); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	if !slices.Equal(w.dials, []string{"other.local:9000"}) {
		t.Fatalf("dials %v", w.dials)
	}
}

func TestARouteDoesNotAllowAnOriginThatEnvJSONDoesNotList(t *testing.T) {
	w := wireWorld(t, `{"target":"http://orders.local:8080","other":"http://listed.local:1"}`)
	if err := os.WriteFile(filepath.Join(w.scripts, "suites", "fake.ts"), []byte(strings.ReplaceAll(wireProbeSuite, `String(ctx.env.target) + "/probe"`, `"http://unlisted.local:8080/probe"`)), 0o644); err != nil {
		t.Fatal(err)
	}
	got := w.run(recording("unlisted=unlisted.local:8080=cockpit-wire:41001"))
	var result contract.Result
	w.read("result.json", &result)
	if got.code != 1 || result.Steps[0].Status != contract.StepStatusFailed || !strings.Contains(result.Steps[0].Message, "address not allowed") {
		t.Fatalf("exit %d step %+v", got.code, result.Steps[0])
	}
	if len(w.dials) != 0 {
		t.Fatalf("a refused origin was dialed: %v", w.dials)
	}
}

func TestThePlaceholderExpandsToTheProxyOnlyForARouteInARecordingRun(t *testing.T) {
	env := map[string]any{
		"direct":  "http://${COCKPIT_WIRE_ORDERS:-orders:8080}/hooks",
		"unknown": "${COCKPIT_WIRE_NOPE:-fallback}",
		"other":   "${OTHER:-kept} ${COCKPIT_WIRE_ROUTES} $COCKPIT_WIRE_ORDERS",
		"nested":  map[string]any{"list": []any{"${COCKPIT_WIRE_ORDERS:-a}", 7.0}},
		"number":  7.0,
	}
	routes, err := parseWireRoutes("orders=orders:8080=cockpit-wire:41001")
	if err != nil {
		t.Fatal(err)
	}
	got := routes.expandEnv(env)
	if got["direct"] != "http://cockpit-wire:41001/hooks" || got["unknown"] != "fallback" || got["number"] != 7.0 {
		t.Fatalf("expanded %v", got)
	}
	if got["other"] != env["other"] {
		t.Fatalf("a value that is not the placeholder changed: %v", got["other"])
	}
	if list := got["nested"].(map[string]any)["list"].([]any); list[0] != "cockpit-wire:41001" || list[1] != 7.0 {
		t.Fatalf("nested %v", list)
	}
	empty, err := parseWireRoutes("")
	if err != nil {
		t.Fatal(err)
	}
	if off := empty.expandEnv(env); off["direct"] != "http://orders:8080/hooks" || off["unknown"] != "fallback" {
		t.Fatalf("without recording %v", off)
	}
}

func TestAnExpandedEnvValueIsAnAllowedOriginAndItsDefaultRunsWithoutRecording(t *testing.T) {
	env := `{"target":"http://${COCKPIT_WIRE_ORDERS:-orders.local:8080}"}`
	recorded := wireWorld(t, env)
	if got := recorded.run(recording("orders=orders.local:8080=cockpit-wire:41001")); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	if !slices.Equal(recorded.dials, []string{"cockpit-wire:41001"}) {
		t.Fatalf("recording dials %v", recorded.dials)
	}
	plain := wireWorld(t, env)
	if got := plain.run(nil); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	if !slices.Equal(plain.dials, []string{"orders.local:8080"}) {
		t.Fatalf("plain dials %v", plain.dials)
	}
}

func TestAMalformedRouteListExitsTwoBeforeAnyRequest(t *testing.T) {
	refused := 0
	for _, value := range []string{"orders", "orders=orders:8080", "=orders:8080=cockpit-wire:41001", "orders=orders=cockpit-wire:41001", "a=b:1=c:2,bad"} {
		w := wireWorld(t, `{"target":"http://orders.local:8080"}`)
		got := w.run(recording(value))
		if got.code != 2 || len(w.dials) != 0 || !strings.Contains(strings.Join(got.stderr, "\n"), contract.WireRoutesVariable) {
			t.Errorf("%q: exit %d stderr %v dials %v", value, got.code, got.stderr, w.dials)
		}
		refused++
	}
	if refused != 5 {
		t.Fatalf("%d cases", refused)
	}
}

func TestTheRoutesTheHarnessRendersAreTheRoutesTheRunnerReads(t *testing.T) {
	decl := contract.Declaration{Wire: contract.WireSection{Routes: []contract.WireRoute{{Name: "ledger-api", Service: "Ledger", Port: 8081}, {Name: "orders", Service: "orders", Port: 8080}}}}
	routes := compose.WireRoutes(decl)
	parsed, err := parseWireRoutes(compose.WireRunnerRoutes(routes))
	if err != nil {
		t.Fatal(err)
	}
	reroute := parsed.rerouter()
	checked := 0
	for _, r := range routes {
		if got := reroute(r.Upstream()); got != r.Proxy() {
			t.Errorf("%s reroutes to %s, want %s", r.Upstream(), got, r.Proxy())
		}
		if got := parsed.proxyByVariable[r.Variable()]; got != r.Proxy() {
			t.Errorf("%s holds %s, want %s", r.Variable(), got, r.Proxy())
		}
		checked++
	}
	if checked != 2 || reroute("elsewhere:1") != "elsewhere:1" {
		t.Fatalf("checked %d, an unrouted address read %s", checked, reroute("elsewhere:1"))
	}
}

package compose

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	recorder "github.com/d0lim/cockpit/internal/runner/wire"
)

var wireHarnessMutations = []porttest.Mutation{
	{"a run that does not record renders the harness of before the feature", "always add the recorder in Harness", "red: the toy harness differs from the golden file", "TestAHarnessWithoutWireIsByteForByteTheHarnessBeforeTheFeature"},
	{"the recorder is confined like every harness service", "skip hardened for the recorder", "red: the recorder runs with a writable root and capabilities", "TestARecordingRunGetsAConfinedRecorderOnTheInternalNetworksOnly"},
	{"the recorder joins only the internal networks", "append egress to its networks", "red: the recorder joins egress", "TestARecordingRunGetsAConfinedRecorderOnTheInternalNetworksOnly"},
	{"every consumer service except the builder waits for the healthy recorder", "never set depends_on", "red: app starts without waiting for the recorder", "TestEveryConsumerServiceExceptTheBuilderWaitsForTheHealthyRecorder"},
	{"the builder does not wait for the recorder", "set depends_on on the builder too", "red: the builder waits for a service of the run stage", "TestEveryConsumerServiceExceptTheBuilderWaitsForTheHealthyRecorder"},
	{"the runner learns the routes", "drop COCKPIT_WIRE_ROUTES from the runner environment", "red: the runner environment lacks the routes", "TestTheRunnerGetsTheRoutesBesideItsOwnEnvironment"},
	{"WireRoutes is the single source of ports", "start the listen ports at another number", "red: the first listen port is not 41001", "TestWireRoutesNumberTheListenPortsAndEveryOutputAgrees"},
	{"a recording run needs routes", "skip the empty-routes refusal", "red: a recorder without routes renders", "TestARecordingRunWithoutRoutesIsRefused"},
	{"run.env names the proxy of each route", "drop the wire lines from RunEnv", "red: run.env lacks COCKPIT_WIRE_LEDGER", "TestRunEnvCarriesOneLinePerRouteOnlyForARecordingRun"},
}

func TestWireHarnessMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "harness_wire_test.go", wireHarnessMutations)
}

const (
	harnessGolden = "testdata/harness.toy.golden.yaml"
	updateGolden  = "COCKPIT_UPDATE_HARNESS_GOLDEN"
)

var testWireSection = contract.WireSection{
	Routes:       []contract.WireRoute{{Name: "ledger-api", Service: "app", Port: 8081}, {Name: "orders", Service: "app", Port: 8080}},
	PublicFields: []contract.WirePublicField{{Key: "token", Value: "tok-[0-9a-f]{8}$"}},
}

func goldenInput(t *testing.T) HarnessInput {
	t.Helper()
	layout, err := NewLayout("/fixture/runs/r-toy")
	if err != nil {
		t.Fatal(err)
	}
	return HarnessInput{RunID: "r-toy", RunnerImage: "cockpit-runner:0123456789ab", PortBase: 22000, Layout: layout, Loaded: mustLoadToy(t)}
}

func recordingDoc(t *testing.T, mutate func(string)) (map[string]any, HarnessInput) {
	t.Helper()
	loaded, err := loadToy(t, mutate)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Declaration.Wire = testWireSection
	doc, _, in := harnessDocFrom(t, HarnessInput{RunID: runID, RunnerImage: "cockpit-runner:0123456789ab", PortBase: 22100, Layout: testLayout(t), Loaded: loaded, Wire: true})
	return doc, in
}

func TestAHarnessWithoutWireIsByteForByteTheHarnessBeforeTheFeature(t *testing.T) {
	in := goldenInput(t)
	got, err := Harness(in)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv(updateGolden) == "1" {
		if err := os.MkdirAll(filepath.Dir(harnessGolden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(harnessGolden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(harnessGolden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) || len(want) == 0 {
		t.Fatalf("the harness differs from %s (%d bytes, want %d)", harnessGolden, len(got), len(want))
	}
	in.Loaded.Declaration.Wire = testWireSection
	declared, err := Harness(in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(declared, want) {
		t.Fatal("declaring wire routes changed the harness of a run that does not record")
	}
}

func TestARecordingRunGetsAConfinedRecorderOnTheInternalNetworksOnly(t *testing.T) {
	doc, in := recordingDoc(t, withNetwork(t))
	svc := service(t, doc, RoleWire)
	if svc["image"] != in.RunnerImage || svc["pull_policy"] != "never" || svc["labels"].(map[string]any)[LabelRole] != RoleWire {
		t.Fatalf("recorder %v", svc)
	}
	if svc["read_only"] != true || strings.Join(strs(svc["cap_drop"]), ",") != "ALL" || svc["pids_limit"] == nil || svc["mem_limit"] == nil {
		t.Fatalf("the recorder is not confined: %v", svc)
	}
	if got := strings.Join(strs(svc["networks"]), ","); got != "default,backplane" {
		t.Fatalf("recorder networks %s", got)
	}
	for _, field := range []string{"ports", "volumes", "tmpfs", "profiles", "environment", "entrypoint"} {
		if _, ok := svc[field]; ok {
			t.Errorf("the recorder sets %s: %v", field, svc[field])
		}
	}
	health := svc["healthcheck"].(map[string]any)
	if strings.Join(strs(health["test"]), " ") != "CMD /cockpit-runner wire-ctl ping" || health["interval"] != "1s" || health["timeout"] != "1s" || health["retries"] != 30 {
		t.Fatalf("healthcheck %v", health)
	}
}

func TestEveryConsumerServiceExceptTheBuilderWaitsForTheHealthyRecorder(t *testing.T) {
	doc, in := recordingDoc(t, nil)
	waiting := 0
	for _, name := range in.Loaded.Model.ServiceNames() {
		deps, ok := service(t, doc, name)["depends_on"].(map[string]any)
		if name == in.Loaded.Declaration.Project.Builder {
			if ok {
				t.Errorf("the builder waits for %v", deps)
			}
			continue
		}
		if !ok || deps[RoleWire].(map[string]any)["condition"] != "service_healthy" {
			t.Errorf("%s depends_on %v", name, deps)
			continue
		}
		waiting++
	}
	if waiting != len(in.Loaded.Model.Services)-1 || waiting == 0 {
		t.Fatalf("%d services wait", waiting)
	}
	for _, role := range HarnessServices {
		if _, ok := service(t, doc, role)["depends_on"]; ok {
			t.Errorf("harness service %s waits for the recorder", role)
		}
	}
}

func TestTheRunnerGetsTheRoutesBesideItsOwnEnvironment(t *testing.T) {
	doc, in := recordingDoc(t, nil)
	env := service(t, doc, RoleRunner)["environment"].(map[string]any)
	want := "ledger-api=app:8081=cockpit-wire:41001,orders=app:8080=cockpit-wire:41002"
	if env[contract.WireRoutesVariable] != want {
		t.Fatalf("routes %v, want %s", env[contract.WireRoutesVariable], want)
	}
	if env["COCKPIT_RUNNER_IMAGE"] != in.RunnerImage || env["E2E_EXEC_OUT"] != "${E2E_EXEC_OUT:-}" || env["COCKPIT_TRACE_ID"] != "${COCKPIT_TRACE_ID:-}" || len(env) != 4 {
		t.Fatalf("runner environment %v", env)
	}
	app := service(t, doc, "app")["environment"].(map[string]any)
	if app["OTEL_EXPORTER_OTLP_ENDPOINT"] == nil {
		t.Fatalf("a traced service lost its OTLP variables: %v", app)
	}
}

func TestWireRoutesNumberTheListenPortsAndEveryOutputAgrees(t *testing.T) {
	doc, in := recordingDoc(t, nil)
	routes := WireRoutes(in.Loaded.Declaration)
	if len(routes) != 2 || routes[0].Listen != 41001 || routes[1].Listen != 41002 || routes[0].Variable() != "COCKPIT_WIRE_LEDGER_API" || routes[1].Proxy() != "cockpit-wire:41002" {
		t.Fatalf("routes %+v", routes)
	}
	command := strs(service(t, doc, RoleWire)["command"])
	encoded := base64.RawURLEncoding.EncodeToString([]byte("tok-[0-9a-f]{8}$"))
	want := []string{"wire", "--project", runID, "--public-field", "token=" + encoded, "41001=app:8081", "41002=app:8080"}
	if !slices.Equal(command, want) {
		t.Fatalf("command %q, want %q", command, want)
	}
	for i, spec := range command[5:] {
		parsed, err := recorder.ParseRoute(spec)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Listen != routes[i].Listen || parsed.Service != routes[i].Service || parsed.Port != routes[i].Port {
			t.Errorf("%s reads %+v, want %+v", spec, parsed, routes[i])
		}
	}
}

func TestARecordingRunWithoutRoutesIsRefused(t *testing.T) {
	in := goldenInput(t)
	in.Wire = true
	if _, err := Harness(in); err == nil || !strings.Contains(err.Error(), "wire route") {
		t.Fatalf("a recording run without routes: %v", err)
	}
	in.Loaded.Declaration.Wire = testWireSection
	if _, err := Harness(in); err != nil {
		t.Fatalf("a recording run with routes: %v", err)
	}
}

func TestRunEnvCarriesOneLinePerRouteOnlyForARecordingRun(t *testing.T) {
	l := testLayout(t)
	plain, err := RunEnv(RunEnvInput{RunID: runID, Layout: l})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "COCKPIT_WIRE_") {
		t.Fatalf("run.env without wire: %s", plain)
	}
	decl := contract.Declaration{Wire: contract.WireSection{Routes: []contract.WireRoute{{Name: "ledger", Service: "ledger", Port: 8081}, {Name: "order-api", Service: "orders", Port: 8080}}}}
	text, err := RunEnv(RunEnvInput{RunID: runID, Layout: l, Wire: WireRoutes(decl)})
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range []string{"COCKPIT_WIRE_LEDGER=cockpit-wire:41001", "COCKPIT_WIRE_ORDER_API=cockpit-wire:" + strconv.Itoa(41002)} {
		if !strings.Contains(text, line+"\n") {
			t.Errorf("line %d %q is missing from %s", i, line, text)
		}
	}
}

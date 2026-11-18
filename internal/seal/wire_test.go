package seal

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var wireMutations = []porttest.Mutation{
	{"the recorder is a required role of a recording run only", "pass false for the wire flag in roles()", "red: a recording run without the recorder keeps roles.present green", "TestTheWireRoleIsRequiredOnlyWhenTheRunRecords"},
	{"a recorder the harness renders passes", "make wireCheck report a violation always", "red: the harness render fails wire.routes", "TestTheRecorderTheHarnessRendersPassesEveryAssertion"},
	{"each clause of wire.routes catches its own violation", "delete that clause from wireCheck or wireRouteProblems", "red: that case stays green", "TestEachBrokenRecorderTurnsOnlyWireRoutesRed"},
	{"a recorder in a run that does not record is a violation", "skip the loop over services when Wire is off", "red: the named recorder passes", "TestARecorderInARunThatDoesNotRecordTurnsOnlyWireRoutesRed"},
	{"the recorder is confined by the common assertions too", "remove compose.RoleWire from compose.HarnessServices", "red: a bind under src passes mounts.binds", "TestARecorderThatPublishesBindsJoinsEgressOrIsTargetedByTheEdgeTurnsTheCommonAssertionRed"},
}

func TestWireMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "wire_test.go", wireMutations)
}

var wireTestRoutes = []compose.WireRoute{{Name: "app", Service: "app", Port: 8080, Listen: 41001}}

func recorderService() map[string]any {
	return map[string]any{
		"image":   fixtureRunner,
		"command": []any{"wire", "--project", fixtureRunID, "--public-field", "token=dG9r", "41001=app:8080"},
		"labels":  map[string]any{compose.LabelManaged: "true", compose.LabelRunID: fixtureRunID, compose.LabelRole: compose.RoleWire},
		"networks": map[string]any{
			"default": nil,
		},
	}
}

func recordingInput(t *testing.T) Input {
	t.Helper()
	in := toyInput(t)
	in.Wire = true
	in.WireRoutes = wireTestRoutes
	record(in.Config["services"])[compose.RoleWire] = recorderService()
	return in
}

func recorderOf(t *testing.T, in Input) map[string]any { return svc(t, in, compose.RoleWire) }

func TestTheRecorderTheHarnessRendersPassesEveryAssertion(t *testing.T) {
	in := toyInput(t)
	raw := map[string]string{"app": "", "db": ""}
	model := compose.Model{Services: raw, Volumes: map[string]bool{}, Networks: map[string]bool{}}
	decl := contract.Declaration{Wire: contract.WireSection{Routes: []contract.WireRoute{{Name: "app", Service: "app", Port: 8080}}, PublicFields: []contract.WirePublicField{{Key: "token", Value: "tok-[0-9a-f]{8}"}}}}
	data, err := compose.Harness(compose.HarnessInput{RunID: fixtureRunID, RunnerImage: fixtureRunner, PortBase: 22000, Layout: in.Layout, Loaded: compose.Loaded{Declaration: decl, Model: model}, Wire: true})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	asJSON, err := json.Marshal(doc.Services[compose.RoleWire])
	if err != nil {
		t.Fatal(err)
	}
	var recorder map[string]any
	if err := json.Unmarshal(asJSON, &recorder); err != nil {
		t.Fatal(err)
	}
	networks := map[string]any{}
	for _, name := range list(recorder["networks"]) {
		networks[text(name)] = nil
	}
	recorder["networks"] = networks
	record(in.Config["services"])[compose.RoleWire] = recorder
	in.Wire = true
	in.WireRoutes = compose.WireRoutes(decl)
	results := Static(in)
	if got := failing(results); len(got) != 0 {
		t.Fatalf("the rendered recorder fails %v: %+v", got, results)
	}
}

func TestEachBrokenRecorderTurnsOnlyWireRoutesRed(t *testing.T) {
	setCommand := func(in *Input, args ...any) { recorderOf(t, *in)["command"] = args }
	cases := map[string]struct {
		says   string
		mutate func(in *Input)
	}{
		"another image":       {"instead of the runner image", func(in *Input) { recorderOf(t, *in)["image"] = "elsewhere:1" }},
		"entrypoint override": {"entrypoint", func(in *Input) { recorderOf(t, *in)["entrypoint"] = []any{"/bin/sh"} }},
		"a named volume": {"volumes", func(in *Input) {
			recorderOf(t, *in)["volumes"] = []any{map[string]any{"type": "volume", "source": "db-data", "target": "/data"}}
		}},
		"tmpfs":           {"tmpfs", func(in *Input) { recorderOf(t, *in)["tmpfs"] = []any{"/tmp"} }},
		"another mode":    {"wire mode", func(in *Input) { setCommand(in, "edge", "41001=app:8080") }},
		"no command":      {"wire mode", func(in *Input) { delete(recorderOf(t, *in), "command") }},
		"another project": {"names project", func(in *Input) { setCommand(in, "wire", "--project", "r-other", "41001=app:8080") }},
		"a route missing": {"declared and not proxied", func(in *Input) { setCommand(in, "wire", "--project", fixtureRunID) }},
		"an extra route":  {"not a declared route", func(in *Input) { setCommand(in, "wire", "--project", fixtureRunID, "41001=app:8080", "41002=db:5432") }},
		"a moved route":   {"not a declared route", func(in *Input) { setCommand(in, "wire", "--project", fixtureRunID, "41002=app:8080") }},
		"a broken route":  {"is not <port>", func(in *Input) { setCommand(in, "wire", "--project", fixtureRunID, "nonsense") }},
		"a host outside the project": {"not a service of this project", func(in *Input) {
			in.WireRoutes = []compose.WireRoute{{Name: "x", Service: "example.com", Port: 443, Listen: 41001}}
			setCommand(in, "wire", "--project", fixtureRunID, "41001=example.com:443")
		}},
		"a harness target": {"targets a harness service", func(in *Input) {
			in.WireRoutes = []compose.WireRoute{{Name: "x", Service: compose.RoleEdge, Port: 80, Listen: 41001}}
			setCommand(in, "wire", "--project", fixtureRunID, "41001="+compose.RoleEdge+":80")
		}},
	}
	if got := failing(Static(recordingInput(t))); len(got) != 0 {
		t.Fatalf("the unbroken recorder fails %v", got)
	}
	evaluated := 0
	for name, c := range cases {
		in := recordingInput(t)
		c.mutate(&in)
		results := Static(in)
		if got := failing(results); !slices.Equal(got, []string{"wire.routes"}) {
			t.Errorf("%s turned %v red", name, got)
			continue
		}
		if detail := results[len(results)-1].Detail; !strings.Contains(detail, c.says) {
			t.Errorf("%s: detail %q lacks %q", name, detail, c.says)
		}
		evaluated++
	}
	if evaluated != 13 {
		t.Fatalf("evaluated %d of %d cases", evaluated, len(cases))
	}
}

func TestARecorderInARunThatDoesNotRecordTurnsOnlyWireRoutesRed(t *testing.T) {
	cases := map[string]func(in *Input){
		"under its own name": func(in *Input) {
			delete(recorderOf(t, *in)["labels"].(map[string]any), compose.LabelRole)
		},
		"holding the role": func(in *Input) {},
	}
	evaluated := 0
	for name, mutate := range cases {
		in := recordingInput(t)
		in.Wire, in.WireRoutes = false, nil
		mutate(&in)
		if got := failing(Static(in)); !slices.Equal(got, []string{"wire.routes"}) {
			t.Errorf("%s turned %v red", name, got)
		}
		evaluated++
	}
	in := toyInput(t)
	if got := failing(Static(in)); len(got) != 0 {
		t.Fatalf("a run without a recorder fails %v", got)
	}
	if evaluated != 2 {
		t.Fatalf("evaluated %d cases", evaluated)
	}
}

func TestTheWireRoleIsRequiredOnlyWhenTheRunRecords(t *testing.T) {
	in := toyInput(t)
	in.Wire = true
	in.WireRoutes = wireTestRoutes
	if got := failing(Static(in)); !slices.Equal(got, []string{"roles.present", "wire.routes"}) {
		t.Fatalf("a recording run without the recorder turned %v red", got)
	}
	unlabelled := recordingInput(t)
	delete(recorderOf(t, unlabelled)["labels"].(map[string]any), compose.LabelRole)
	if got := failing(Static(unlabelled)); !slices.Equal(got, []string{"roles.present"}) {
		t.Fatalf("an unlabelled recorder turned %v red", got)
	}
	if got := failing(Static(recordingInput(t))); len(got) != 0 {
		t.Fatalf("the complete recording run fails %v", got)
	}
}

func TestARecorderThatPublishesBindsJoinsEgressOrIsTargetedByTheEdgeTurnsTheCommonAssertionRed(t *testing.T) {
	cases := map[string]struct {
		mutate func(in *Input)
		red    []string
	}{
		"a published port": {func(in *Input) {
			recorderOf(t, *in)["ports"] = []any{map[string]any{"host_ip": "127.0.0.1", "published": "41001", "target": 41001.0}}
		}, []string{"services.ports"}},
		"a bind": {func(in *Input) {
			recorderOf(t, *in)["volumes"] = []any{map[string]any{"type": "bind", "source": fixtureRun + "/src/db", "target": "/data"}}
		}, []string{"mounts.binds", "wire.routes"}},
		"egress":                        {func(in *Input) { record(recorderOf(t, *in)["networks"])["egress"] = nil }, []string{"services.networks"}},
		"an edge route to the recorder": {func(in *Input) { setRoute(t, *in, 0, "30000=cockpit-wire:41001") }, []string{"edge.upstreams"}},
	}
	evaluated := 0
	for name, c := range cases {
		in := recordingInput(t)
		c.mutate(&in)
		if got := failing(Static(in)); !slices.Equal(got, c.red) {
			t.Errorf("%s turned %v red, want %v", name, got, c.red)
		}
		evaluated++
	}
	if evaluated != 4 {
		t.Fatalf("evaluated %d cases", evaluated)
	}
}

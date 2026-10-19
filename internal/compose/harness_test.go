package compose

import (
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var harnessMutations = []porttest.Mutation{
	{"default and consumer networks are internal", "drop internal: true from the generated networks", "red: default or the consumer network is not internal", "TestHarnessMakesDefaultAndConsumerNetworksInternalAndOnlyEdgeProbeControlAndBuilderJoinEgress"},
	{"only the edge, probe-control and builder reach egress", "add egress to the runner networks", "red: the runner joins egress", "TestHarnessMakesDefaultAndConsumerNetworksInternalAndOnlyEdgeProbeControlAndBuilderJoinEgress"},
	{"edge ports are published on loopback only", "publish edge ports without the 127.0.0.1 prefix", "red: an edge port binds every interface", "TestEdgePublishesEveryDeclaredOffsetOnLoopbackAndProxiesToTheDeclaredServicePort"},
	{"consumer services publish no ports", "drop the !reset ports entry for consumer services", "red: a consumer service keeps its own ports", "TestConsumerServicesLoseTheirPortsAndCarryRunLabelsAndTheBuilderItsRole"},
	{"runner mounts only work tree paths and only e2e is writable", "bind the run source tree into the runner", "red: a bind outside work/ or a writable scripts bind appears", "TestRunnerBindsOnlyWorkTreePathsAndOnlyE2EIsWritable"},
	{"harness containers are confined", "drop read_only and cap_drop from hardened", "red: a harness service runs with a writable root or capabilities", "TestHarnessServicesRunReadOnlyWithoutCapabilitiesAndWithLimits"},
}

func TestHarnessMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "harness_test.go", harnessMutations)
}

const runID = "r-toy-01"

func harnessDoc(t *testing.T, mutate func(dir string)) (map[string]any, yaml.Node, HarnessInput) {
	t.Helper()
	loaded, err := loadToy(t, mutate)
	if err != nil {
		t.Fatal(err)
	}
	return harnessDocFrom(t, HarnessInput{RunID: runID, RunnerImage: "cockpit-runner:0123456789ab", PortBase: 22100, Layout: testLayout(t), Loaded: loaded})
}

func harnessDocFrom(t *testing.T, in HarnessInput) (map[string]any, yaml.Node, HarnessInput) {
	t.Helper()
	data, err := Harness(in)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		t.Fatal(err)
	}
	if err := node.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	return doc, node, in
}

func service(t *testing.T, doc map[string]any, name string) map[string]any {
	t.Helper()
	svc, ok := doc["services"].(map[string]any)[name].(map[string]any)
	if !ok {
		t.Fatalf("service %s is missing", name)
	}
	return svc
}

func strs(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func withNetwork(t *testing.T) func(string) {
	return func(d string) {
		replaceIn(t, d+"/compose.yaml", "volumes:\n  db-data:\n", "volumes:\n  db-data:\nnetworks:\n  backplane: {}\n  shared:\n    external: true\n")
	}
}

func TestHarnessMakesDefaultAndConsumerNetworksInternalAndOnlyEdgeProbeControlAndBuilderJoinEgress(t *testing.T) {
	doc, node, _ := harnessDoc(t, withNetwork(t))
	nets := doc["networks"].(map[string]any)
	for _, name := range []string{DefaultNetwork, "backplane"} {
		if nets[name].(map[string]any)["internal"] != true {
			t.Errorf("network %s is not internal", name)
		}
	}
	if _, ok := nets["shared"]; ok {
		t.Error("the harness redefined an external network")
	}
	if _, ok := nets[EgressNetwork].(map[string]any)["internal"]; ok {
		t.Error("egress is internal")
	}
	joined := map[string]bool{}
	for name, raw := range doc["services"].(map[string]any) {
		nets, ok := raw.(map[string]any)["networks"].([]any)
		if ok && strings.Contains(strings.Join(strs(nets), ","), EgressNetwork) {
			joined[name] = true
		}
	}
	if len(joined) != 3 || !joined[RoleEdge] || !joined[RoleProbeControl] || !joined["builder"] {
		t.Fatalf("services on egress: %v", joined)
	}
	if got := strings.Join(strs(service(t, doc, RoleRunner)["networks"]), ","); got != "default,backplane" {
		t.Fatalf("runner networks %s", got)
	}
	if got := strings.Join(strs(service(t, doc, RoleProbe)["networks"]), ","); got != "default" {
		t.Fatalf("probe networks %s", got)
	}
	if tag := serviceField(t, node, "builder", "networks").Tag; tag != "!override" {
		t.Fatalf("builder networks tag %q", tag)
	}
}

func serviceField(t *testing.T, root yaml.Node, svc, field string) *yaml.Node {
	t.Helper()
	n := lookup(lookup(lookup(root.Content[0], "services"), svc), field)
	if n == nil {
		t.Fatalf("services.%s.%s is missing", svc, field)
	}
	return n
}

func TestEdgePublishesEveryDeclaredOffsetOnLoopbackAndProxiesToTheDeclaredServicePort(t *testing.T) {
	doc, _, in := harnessDoc(t, nil)
	edge := service(t, doc, RoleEdge)
	ports := strs(edge["ports"])
	command := strs(edge["command"])
	decl := in.Loaded.Declaration
	if len(ports) != len(decl.Ports) || len(decl.Ports) == 0 || len(command) != len(decl.Ports)+1 || command[0] != "edge" {
		t.Fatalf("ports %v command %v", ports, command)
	}
	for i, p := range decl.Ports {
		listen := strconv.Itoa(30000 + p.Offset)
		want := "127.0.0.1:" + strconv.Itoa(in.PortBase+p.Offset) + ":" + listen
		if ports[i] != want {
			t.Errorf("port %d: %s, want %s", i, ports[i], want)
		}
		if route := listen + "=" + p.Service + ":" + strconv.Itoa(p.Port); command[i+1] != route {
			t.Errorf("route %d: %s, want %s", i, command[i+1], route)
		}
	}
	if edge["image"] != in.RunnerImage || edge["pull_policy"] != "never" {
		t.Fatalf("edge image %v pull %v", edge["image"], edge["pull_policy"])
	}
	probe := service(t, doc, RoleProbe)
	if got := strings.Join(strs(probe["command"]), " "); got != "probe "+strconv.Itoa(in.PortBase+CanaryOffset) {
		t.Fatalf("probe command %s", got)
	}
}

func TestConsumerServicesLoseTheirPortsAndCarryRunLabelsAndTheBuilderItsRole(t *testing.T) {
	doc, node, in := harnessDoc(t, nil)
	evaluated := 0
	for _, name := range in.Loaded.Model.ServiceNames() {
		ports := serviceField(t, node, name, "ports")
		if ports.Tag != "!reset" || len(ports.Content) != 0 {
			t.Errorf("%s ports %q %d", name, ports.Tag, len(ports.Content))
		}
		labels := service(t, doc, name)["labels"].(map[string]any)
		if labels[LabelManaged] != "true" || labels[LabelRunID] != runID {
			t.Errorf("%s labels %v", name, labels)
		}
		evaluated++
	}
	if evaluated != 5 {
		t.Fatalf("evaluated %d consumer services", evaluated)
	}
	if role := service(t, doc, "builder")["labels"].(map[string]any)[LabelRole]; role != RoleBuilder {
		t.Fatalf("builder role %v", role)
	}
	for _, role := range HarnessServices {
		if got := service(t, doc, role)["labels"].(map[string]any)[LabelRole]; got != role {
			t.Errorf("%s role label %v", role, got)
		}
	}
	vols := doc["volumes"].(map[string]any)
	if vols["db-data"].(map[string]any)["labels"].(map[string]any)[LabelRunID] != runID {
		t.Fatalf("volume labels %v", vols)
	}
}

func TestRunnerBindsOnlyWorkTreePathsAndOnlyE2EIsWritable(t *testing.T) {
	doc, _, in := harnessDoc(t, nil)
	binds := service(t, doc, RoleRunner)["volumes"].([]any)
	writable := 0
	for _, raw := range binds {
		b := raw.(map[string]any)
		if b["type"] != "bind" || !strings.HasPrefix(b["source"].(string), in.Layout.Work+"/") {
			t.Errorf("bind %v is outside the work tree", b)
		}
		if b["read_only"] != true {
			writable++
			if b["target"] != "/work/e2e" {
				t.Errorf("writable bind %v", b)
			}
		}
	}
	if len(binds) != 4 || writable != 1 {
		t.Fatalf("%d binds, %d writable", len(binds), writable)
	}
	env := service(t, doc, RoleRunner)["environment"].(map[string]any)
	if env["COCKPIT_RUNNER_IMAGE"] != in.RunnerImage {
		t.Fatalf("runner environment %v", env)
	}
}

func TestHarnessServicesRunReadOnlyWithoutCapabilitiesAndWithLimits(t *testing.T) {
	doc, _, _ := harnessDoc(t, nil)
	for _, role := range HarnessServices {
		svc := service(t, doc, role)
		if svc["read_only"] != true || strings.Join(strs(svc["cap_drop"]), ",") != "ALL" || svc["pids_limit"] == nil || svc["mem_limit"] == nil {
			t.Errorf("%s is not confined: %v", role, svc)
		}
		if strings.Join(strs(svc["security_opt"]), ",") != "no-new-privileges:true" {
			t.Errorf("%s security_opt %v", role, svc["security_opt"])
		}
	}
	if _, err := Harness(HarnessInput{RunID: runID}); err == nil {
		t.Fatal("a harness without a runner image was generated")
	}
}

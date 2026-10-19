package compose

import (
	"fmt"
	"strconv"

	"go.yaml.in/yaml/v3"

	"github.com/d0lim/cockpit/internal/contract"
)

const (
	LabelManaged     = "io.cockpit.managed"
	LabelRunID       = "io.cockpit.run-id"
	LabelRole        = "io.cockpit.role"
	RoleEdge         = "edge"
	RoleRunner       = "runner"
	RoleProbe        = "probe"
	RoleProbeControl = "probe-control"
	RoleBuilder      = "builder"
	DefaultNetwork   = "default"
	EgressNetwork    = "egress"
	runnerMemLimit   = "1g"
	runnerPidsLimit  = 512
)

var HarnessServices = []string{RoleEdge, RoleRunner, RoleProbe, RoleProbeControl}

type HarnessInput struct {
	RunID       string
	RunnerImage string
	PortBase    int
	Layout      Layout
	Loaded      Loaded
}

type tagged struct {
	tag   string
	value any
}

func (t tagged) MarshalYAML() (any, error) {
	var n yaml.Node
	if err := n.Encode(t.value); err != nil {
		return nil, err
	}
	n.Tag = t.tag
	return &n, nil
}

type bind struct {
	Type     string `yaml:"type"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"read_only,omitempty"`
}

func labels(runID, role string) map[string]string {
	l := map[string]string{LabelManaged: "true", LabelRunID: runID}
	if role != "" {
		l[LabelRole] = role
	}
	return l
}

func hardened(svc map[string]any) map[string]any {
	svc["read_only"] = true
	svc["cap_drop"] = []string{"ALL"}
	svc["security_opt"] = []string{"no-new-privileges:true"}
	svc["pids_limit"] = runnerPidsLimit
	svc["mem_limit"] = runnerMemLimit
	return svc
}

func Harness(in HarnessInput) ([]byte, error) {
	if in.RunID == "" || in.RunnerImage == "" || in.PortBase <= 0 {
		return nil, fmt.Errorf("harness needs a run id, a runner image and a port base")
	}
	decl, model := in.Loaded.Declaration, in.Loaded.Model
	inner := []string{DefaultNetwork}
	networks := map[string]any{
		DefaultNetwork: map[string]any{"internal": true, "labels": labels(in.RunID, "")},
		EgressNetwork:  map[string]any{"labels": labels(in.RunID, "")},
	}
	for _, name := range sortedKeys(model.Networks) {
		if model.Networks[name] || name == DefaultNetwork {
			continue
		}
		networks[name] = map[string]any{"internal": true, "labels": labels(in.RunID, "")}
		inner = append(inner, name)
	}
	volumes := map[string]any{}
	for _, name := range sortedKeys(model.Volumes) {
		if !model.Volumes[name] {
			volumes[name] = map[string]any{"labels": labels(in.RunID, "")}
		}
	}
	services := map[string]any{}
	for _, name := range model.ServiceNames() {
		svc := map[string]any{"labels": labels(in.RunID, ""), "ports": tagged{"!reset", []string{}}}
		if name == decl.Project.Builder {
			svc["labels"] = labels(in.RunID, RoleBuilder)
			svc["networks"] = tagged{"!override", []string{EgressNetwork}}
		}
		services[name] = svc
	}
	image := func(role string) map[string]any {
		return map[string]any{"image": in.RunnerImage, "pull_policy": "never", "labels": labels(in.RunID, role)}
	}
	edge := hardened(image(RoleEdge))
	edge["command"] = append([]string{"edge"}, edgeRoutes(decl.Ports)...)
	edge["networks"] = append(append([]string{}, inner...), EgressNetwork)
	edge["ports"] = edgePublished(in.PortBase, decl.Ports)
	services[RoleEdge] = edge
	runner := hardened(image(RoleRunner))
	runner["profiles"] = []string{RoleRunner}
	runner["networks"] = inner
	runner["tmpfs"] = []string{"/tmp"}
	runner["volumes"] = runnerBinds(in.Layout, decl.Inputs.RunnerSources)
	runner["environment"] = map[string]string{"COCKPIT_RUNNER_IMAGE": in.RunnerImage, "E2E_EXEC_OUT": "${E2E_EXEC_OUT:-}"}
	services[RoleRunner] = runner
	for role, nets := range map[string][]string{RoleProbe: {DefaultNetwork}, RoleProbeControl: {EgressNetwork}} {
		probe := hardened(image(role))
		probe["profiles"] = []string{RoleProbe}
		probe["networks"] = nets
		probe["command"] = []string{"probe", strconv.Itoa(CanaryPort(in.PortBase))}
		probe["extra_hosts"] = []string{"host.docker.internal:host-gateway"}
		services[role] = probe
	}
	doc := map[string]any{"services": services, "networks": networks}
	if len(volumes) > 0 {
		doc["volumes"] = volumes
	}
	return yaml.Marshal(doc)
}

func runnerBinds(l Layout, sources []contract.RunnerSource) []bind {
	binds := []bind{
		{Type: "bind", Source: l.Work + "/scripts", Target: "/work/scripts", ReadOnly: true},
		{Type: "bind", Source: l.Work + "/" + DeclarationPath, Target: "/work/" + DeclarationPath, ReadOnly: true},
		{Type: "bind", Source: l.Work + "/e2e", Target: "/work/e2e"},
	}
	for _, s := range sources {
		binds = append(binds, bind{Type: "bind", Source: l.Work + "/source/" + string(s.Name), Target: "/work/source/" + string(s.Name), ReadOnly: true})
	}
	return binds
}

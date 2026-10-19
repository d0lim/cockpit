package compose

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/runnerbin"
)

type Model struct {
	Services map[string]string
	Volumes  map[string]bool
	Networks map[string]bool
}

func (m Model) ServiceNames() []string {
	return sortedKeys(m.Services)
}

func LoadModel(ctx context.Context, git Git, tree Tree, files []contract.RelPath) (Model, error) {
	m := Model{Services: map[string]string{}, Volumes: map[string]bool{}, Networks: map[string]bool{}}
	for i, f := range files {
		field := fmt.Sprintf("compose.files[%d]", i)
		raw, err := git.Blob(ctx, tree.Entries[string(f)].ID)
		if err != nil {
			return Model{}, reject(RejectCompose, field, "cannot read %s: %v", f, err)
		}
		if err := m.add(raw); err != nil {
			return Model{}, reject(RejectCompose, field, "%s: %v", f, err)
		}
	}
	return m, nil
}

func ParseModel(files ...[]byte) (Model, error) {
	m := Model{Services: map[string]string{}, Volumes: map[string]bool{}, Networks: map[string]bool{}}
	for _, raw := range files {
		if err := m.add(raw); err != nil {
			return Model{}, err
		}
	}
	return m, nil
}

func (m *Model) add(raw []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("not valid YAML: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil
	}
	root := deref(doc.Content[0])
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("the top level is not a mapping")
	}
	for name, svc := range pairs(lookup(root, "services")) {
		image := ""
		if n := lookup(svc, "image"); n != nil && n.Kind == yaml.ScalarNode {
			image = n.Value
		}
		if prev, ok := m.Services[name]; !ok || image != "" {
			m.Services[name] = image
		} else {
			m.Services[name] = prev
		}
	}
	for name, vol := range pairs(lookup(root, "volumes")) {
		m.Volumes[name] = m.Volumes[name] || external(vol)
	}
	for name, net := range pairs(lookup(root, "networks")) {
		m.Networks[name] = m.Networks[name] || external(net)
	}
	return nil
}

func external(n *yaml.Node) bool {
	e := lookup(n, "external")
	return e != nil && e.Kind == yaml.ScalarNode && e.Value == "true"
}

func deref(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

func lookup(n *yaml.Node, key string) *yaml.Node {
	n = deref(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	var merged *yaml.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		switch k.Value {
		case key:
			return deref(v)
		case "<<":
			if merged == nil {
				merged = lookup(v, key)
			}
		}
	}
	return merged
}

func pairs(n *yaml.Node) map[string]*yaml.Node {
	out := map[string]*yaml.Node{}
	n = deref(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return out
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		out[n.Content[i].Value] = n.Content[i+1]
	}
	return out
}

func usesRunnerRepository(image string) bool {
	ref, _, _ := strings.Cut(image, "@")
	last := ref[strings.LastIndex(ref, "/")+1:]
	repo, _, _ := strings.Cut(last, ":")
	return repo == runnerbin.Repository
}

func (m Model) check(d contract.Declaration) error {
	for _, name := range m.ServiceNames() {
		if usesRunnerRepository(m.Services[name]) {
			return reject(RejectCompose, "services."+name+".image", "%q uses the %s repository, which only the hub may use", m.Services[name], runnerbin.Repository)
		}
	}
	if b := d.Project.Builder; b != "" && !m.has(b) {
		return reject(RejectCompose, "project.builder", "builder service %q is not defined in compose.files", b)
	}
	for i, s := range d.Project.ExpectedServices {
		if !m.has(s) {
			return reject(RejectCompose, fmt.Sprintf("project.expectedServices[%d]", i), "service %q is not defined in compose.files", s)
		}
	}
	for i, p := range d.Ports {
		if !m.has(p.Service) {
			return reject(RejectPorts, fmt.Sprintf("ports[%d].service", i), "service %q is not defined in compose.files", p.Service)
		}
	}
	for _, name := range HarnessServices {
		if m.has(name) {
			return reject(RejectCompose, "services."+name, "service name %q is reserved for the hub harness", name)
		}
	}
	if _, ok := m.Networks[EgressNetwork]; ok {
		return reject(RejectCompose, "networks."+EgressNetwork, "network name %q is reserved for the hub harness", EgressNetwork)
	}
	return nil
}

func (m Model) has(name string) bool {
	_, ok := m.Services[name]
	return ok
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

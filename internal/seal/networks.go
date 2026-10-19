package seal

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
)

func joinedNetworks(v any) []string {
	var out []string
	switch x := v.(type) {
	case []any:
		for _, n := range x {
			out = append(out, fmt.Sprint(n))
		}
	case map[string]any:
		for n := range x {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func internalNetworks(config doc) []string {
	var out []string
	for name, raw := range record(config["networks"]) {
		if name != compose.EgressNetwork && record(raw)["internal"] == true && record(raw)["external"] != true {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func networkChecks(config doc, svcs []service, builder string) []Result {
	nets := record(config["networks"])
	var set []string
	for _, required := range []string{compose.DefaultNetwork, compose.EgressNetwork} {
		if _, ok := nets[required]; !ok {
			set = append(set, "network "+required+" is not declared")
		}
	}
	var notInternal []string
	for _, name := range sortedNames(nets) {
		n := record(nets[name])
		if n["external"] == true {
			set = append(set, "network "+name+" is external")
		}
		if name != compose.EgressNetwork && n["internal"] != true {
			notInternal = append(notInternal, name+" is not internal")
		}
	}
	inner := internalNetworks(config)
	egressOnly := []string{compose.RoleProbeControl}
	if builder != "" {
		egressOnly = append(egressOnly, builder)
	}
	var wrong []string
	for _, s := range svcs {
		actual := joinedNetworks(s.doc["networks"])
		show := strings.Join(actual, ",")
		if show == "" {
			show = "(none)"
		}
		switch {
		case s.name == compose.RoleEdge:
			want := append(slices.Clone(inner), compose.EgressNetwork)
			sort.Strings(want)
			if !slices.Equal(actual, want) {
				wrong = append(wrong, fmt.Sprintf("%s: %s, allowed %s", s.name, show, strings.Join(want, ",")))
			}
		case slices.Contains(egressOnly, s.name):
			if !slices.Equal(actual, []string{compose.EgressNetwork}) {
				wrong = append(wrong, fmt.Sprintf("%s: %s, allowed egress", s.name, show))
			}
		default:
			if len(actual) == 0 || slices.ContainsFunc(actual, func(n string) bool { return !slices.Contains(inner, n) }) {
				wrong = append(wrong, fmt.Sprintf("%s: %s, allowed a subset of %s", s.name, show, strings.Join(inner, ",")))
			}
		}
	}
	return []Result{
		result("networks.set", "the networks are default, egress and project-owned networks; none is external", set),
		result("networks.internal", "every network other than egress is internal", notInternal),
		result("services.networks", "the edge joins every internal network and egress, the builder and probe-control join only egress, every other service joins only internal networks", wrong),
	}
}

func sortedNames(m doc) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

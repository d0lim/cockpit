package compose

import (
	"slices"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var harnessNetworkMutations = []porttest.Mutation{
	{"only the open level leaves project networks routable", "set internal on project networks only when the level is open", "red: a sealed run's default network is not internal", "TestAtNetworkOpenNoProjectNetworkIsInternalAndEveryServiceKeepsItsNetworks"},
	{"open changes the networks and nothing else", "drop egress from the edge at open", "red: the open edge networks differ from the sealed ones", "TestAtNetworkOpenNoProjectNetworkIsInternalAndEveryServiceKeepsItsNetworks"},
}

func TestHarnessNetworkMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "harness_network_test.go", harnessNetworkMutations)
}

func levelDoc(t *testing.T, level contract.NetworkLevel) map[string]any {
	t.Helper()
	loaded, err := loadToy(t, withNetwork(t))
	if err != nil {
		t.Fatal(err)
	}
	loaded.Declaration.Network = level
	doc, _, _ := harnessDocFrom(t, HarnessInput{RunID: runID, RunnerImage: "cockpit-runner:0123456789ab", PortBase: 22100, Layout: testLayout(t), Loaded: loaded})
	return doc
}

func serviceNetworks(doc map[string]any) map[string][]any {
	out := map[string][]any{}
	for name, raw := range doc["services"].(map[string]any) {
		nets, _ := raw.(map[string]any)["networks"].([]any)
		out[name] = nets
	}
	return out
}

func TestAtNetworkOpenNoProjectNetworkIsInternalAndEveryServiceKeepsItsNetworks(t *testing.T) {
	sealed, open := levelDoc(t, contract.NetworkSealed), levelDoc(t, contract.NetworkOpen)
	checked := 0
	for _, name := range []string{DefaultNetwork, "backplane"} {
		if sealed["networks"].(map[string]any)[name].(map[string]any)["internal"] != true {
			t.Errorf("sealed: network %s is not internal", name)
		}
		if _, ok := open["networks"].(map[string]any)[name].(map[string]any)["internal"]; ok {
			t.Errorf("open: network %s sets internal", name)
		}
		checked++
	}
	s, o := serviceNetworks(sealed), serviceNetworks(open)
	for name, nets := range s {
		if !slices.Equal(nets, o[name]) {
			t.Errorf("%s joins %v at open and %v at sealed", name, o[name], nets)
		}
		checked++
	}
	if checked < 6 || len(s) != len(o) {
		t.Fatalf("checked %d, services %d and %d", checked, len(s), len(o))
	}
}

package seal

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var networkMutations = []porttest.Mutation{
	{"only an explicit open level skips networks.internal", "skip networks.internal unless the level is sealed", "red: the zero level passes a render whose default network routes out", "TestTheZeroLevelAndSealedRunNetworksInternalAndOnlyOpenSkipsIt"},
	{"open skips networks.internal and nothing else", "skip services.networks too at open", "red: the open static check returns 23 results and a runner on egress passes", "TestAtNetworkOpenEveryOtherAssertionStillCatchesItsViolation"},
	{"seal.json records the level", "drop Network from the encoded file", "red: seal.json has no network field", "TestSealJSONRecordsTheNetworkLevel"},
}

func TestNetworkMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "network_test.go", networkMutations)
}

func openRender(t *testing.T) Input {
	t.Helper()
	in := toyInput(t)
	in.Network = contract.NetworkOpen
	for _, raw := range record(in.Config["networks"]) {
		delete(record(raw), "internal")
	}
	return in
}

func TestTheZeroLevelAndSealedRunNetworksInternalAndOnlyOpenSkipsIt(t *testing.T) {
	for _, level := range []contract.NetworkLevel{"", contract.NetworkSealed} {
		in := openRender(t)
		in.Network = level
		if got := failing(Static(in)); !slices.Equal(got, []string{"networks.internal"}) {
			t.Errorf("level %q on a render that routes out: failing %v, want only networks.internal", level, got)
		}
	}
	results := Static(openRender(t))
	if got := failing(results); len(got) != 0 || len(results) != 24 || Verdict(results) != contract.SealPassed {
		t.Fatalf("open render at open: %d results, failing %v", len(results), got)
	}
}

func TestAtNetworkOpenEveryOtherAssertionStillCatchesItsViolation(t *testing.T) {
	var names []string
	for _, r := range Static(openRender(t)) {
		names = append(names, r.Name)
	}
	want := slices.DeleteFunc(slices.Clone(assertionNames), func(n string) bool { return n == "networks.internal" })
	if !slices.Equal(names, want) {
		t.Fatalf("open assertions %q, want %q", names, want)
	}
	evaluated := 0
	for name, mutate := range violations(t) {
		if name == "networks.internal" {
			continue
		}
		in := openRender(t)
		mutate(&in)
		if got := failing(Static(in)); !slices.Equal(got, []string{name}) {
			t.Errorf("open: violating copy for %s turned %v red", name, got)
		}
		evaluated++
	}
	if evaluated != 24 {
		t.Fatalf("evaluated %d violating copies, want 24", evaluated)
	}
}

func TestSealJSONRecordsTheNetworkLevel(t *testing.T) {
	for _, level := range []contract.NetworkLevel{contract.NetworkSealed, contract.NetworkOpen} {
		data, err := Encode(Report{Network: level, Static: []Result{{Name: "x", OK: true}}}, time.Unix(0, 0), func(s string) string { return s })
		if err != nil {
			t.Fatal(err)
		}
		var f struct {
			Network contract.NetworkLevel `json:"network"`
		}
		if err := json.Unmarshal(data, &f); err != nil || f.Network != level {
			t.Errorf("level %q encoded as %q (%v)", level, f.Network, err)
		}
	}
}

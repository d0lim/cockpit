package seal

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var normalizeMutations = []porttest.Mutation{
	{"fixture normalisation hides no seal-relevant key", "also delete privileged, ports and volumes in normalizeFixture", "red: the privileges.privileged, services.ports and mounts copies pass after normalisation", "TestFixtureNormalisationDropsOnlyTheDefaultCreateHostPathAndKeepsEverySealViolationVisible"},
}

func TestNormalizeMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "fixture_normalize_test.go", normalizeMutations)
}

func normalizeFixture(config map[string]any) map[string]any {
	for _, raw := range record(config["services"]) {
		svc := record(raw)
		if _, ok := svc["environment"]; ok {
			svc["environment"] = map[string]any{}
		}
		for _, v := range list(svc["volumes"]) {
			bind := record(record(v)["bind"])
			if bind["create_host_path"] == true {
				delete(bind, "create_host_path")
			}
		}
	}
	return config
}

func withComposeDefaults(t *testing.T, in *Input) int {
	t.Helper()
	added := 0
	for _, raw := range record(in.Config["services"]) {
		for _, v := range list(record(raw)["volumes"]) {
			vol := record(v)
			if vol["type"] != "bind" {
				continue
			}
			if _, ok := vol["bind"]; !ok {
				vol["bind"] = map[string]any{}
			}
			record(vol["bind"])["create_host_path"] = true
			added++
		}
	}
	return added
}

func cloneConfig(t *testing.T, config map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestFixtureNormalisationDropsOnlyTheDefaultCreateHostPathAndKeepsEverySealViolationVisible(t *testing.T) {
	cases := violations(t)
	evaluated := 0
	for _, name := range assertionNames {
		in := toyInput(t)
		cases[name](&in)
		if withComposeDefaults(t, &in) < 3 {
			t.Fatalf("%s: fewer than three binds carry create_host_path", name)
		}
		raw := Static(in)
		normalized := in
		normalized.Config = normalizeFixture(cloneConfig(t, in.Config))
		if got, want := failing(Static(normalized)), failing(raw); !slices.Equal(got, want) || !slices.Equal(got, []string{name}) {
			t.Errorf("%s: normalised copy fails %v, raw copy fails %v", name, got, want)
		}
		evaluated++
	}
	if evaluated != len(assertionNames) || evaluated != 24 {
		t.Fatalf("evaluated %d violating copies", evaluated)
	}
	in := toyInput(t)
	withComposeDefaults(t, &in)
	record(list(svc(t, in, "db")["volumes"])[1])["bind"] = map[string]any{"create_host_path": false, "propagation": "rshared"}
	got := normalizeFixture(cloneConfig(t, in.Config))
	bind := record(record(list(record(record(got["services"])["db"])["volumes"])[1])["bind"])
	if bind["create_host_path"] != false || bind["propagation"] != "rshared" {
		t.Fatalf("normalisation removed a non-default bind key: %v", bind)
	}
	if !reflect.DeepEqual(normalizeFixture(cloneConfig(t, toyInput(t).Config)), toyInput(t).Config) {
		t.Fatal("normalisation changed the committed fixture")
	}
}

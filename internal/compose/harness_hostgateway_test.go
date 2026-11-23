package compose

import (
	"slices"
	"sort"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var harnessHostGatewayMutations = []porttest.Mutation{
	{"the harness gives both probes the host-gateway entry", "remove extra_hosts from the probe services in Harness", "red: the probes carry no extra_hosts", "TestHarnessGivesOnlyTheTwoProbesTheHostGatewayEntry"},
	{"on linux the builder writes build/ as the hub's user", "drop the user from the builder in Harness", "red: the builder of a linux run has no user", "TestTheBuilderRunsAsTheHubUserOnLinuxAndKeepsItsImageUserOtherwise"},
}

func TestHarnessHostGatewayMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "harness_hostgateway_test.go", harnessHostGatewayMutations)
}

func TestHarnessGivesOnlyTheTwoProbesTheHostGatewayEntry(t *testing.T) {
	doc, _, _ := harnessDoc(t, nil)
	services := doc["services"].(map[string]any)
	var carrying []string
	for name, raw := range services {
		hosts, ok := raw.(map[string]any)["extra_hosts"]
		if !ok {
			continue
		}
		carrying = append(carrying, name)
		if list, _ := hosts.([]any); len(list) != 1 || list[0] != ProbeHostEntry {
			t.Errorf("%s carries %v", name, hosts)
		}
	}
	sort.Strings(carrying)
	if want := []string{RoleProbe, RoleProbeControl}; !slices.Equal(carrying, want) || len(services) < 4 {
		t.Fatalf("services with extra_hosts %v of %d, want %v", carrying, len(services), want)
	}
}

func TestTheBuilderRunsAsTheHubUserOnLinuxAndKeepsItsImageUserOtherwise(t *testing.T) {
	loaded, err := loadToy(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	builder := loaded.Declaration.Project.Builder
	users := map[string]string{}
	for _, hostUser := range []string{RunnerUser("linux", 1001, 127), RunnerUser("darwin", 501, 20)} {
		doc, _, _ := harnessDocFrom(t, HarnessInput{RunID: runID, RunnerImage: "cockpit-runner:0123456789ab", PortBase: 22100, Layout: testLayout(t), Loaded: loaded, RunnerUser: hostUser})
		services := doc["services"].(map[string]any)
		user, _ := services[builder].(map[string]any)["user"].(string)
		users[hostUser] = user
		if other, ok := services["app"].(map[string]any)["user"]; ok {
			t.Errorf("a consumer service that is not the builder got user %v", other)
		}
	}
	if builder == "" || users["1001:127"] != "1001:127" || users[""] != "" || len(users) != 2 {
		t.Fatalf("builder %q users %v", builder, users)
	}
}

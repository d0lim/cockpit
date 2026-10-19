package compose

import (
	"maps"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var oneShotMutations = []porttest.Mutation{
	{"a service another service waits for with service_completed_successfully is a one-shot", "drop the dependency rule from OneShots", "red: seed reads as long-running", "TestOneShotsAreTheCompletedDependenciesAndTheServicesWithRestartNo"},
	{"a service whose restart is no is a one-shot", "drop the restart rule from OneShots", "red: job reads as long-running", "TestOneShotsAreTheCompletedDependenciesAndTheServicesWithRestartNo"},
	{"unset restart, a restart policy and a healthy or started dependent leave a service long-running", "read unset restart or any dependency as one-shot", "red: app, cache or db reads as one-shot", "TestOneShotsAreTheCompletedDependenciesAndTheServicesWithRestartNo"},
	{"resume skips exactly the one-shots and the profiled services and waits through the one-shots", "build the resume skip set from restart no alone", "red: seed becomes a resident step and app no longer waits for db through it", "TestResumeSkipsTheSameOneShotsThatReadinessReads"},
}

func TestOneShotMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "oneshot_test.go", oneShotMutations)
}

func dep(condition string) map[string]any { return map[string]any{"condition": condition} }

func oneShotConfig() map[string]any {
	return map[string]any{"services": map[string]any{
		"app":        map[string]any{"depends_on": map[string]any{"seed": dep("service_completed_successfully"), "cache": dep("service_started"), "gone": dep("service_completed_successfully")}},
		"web":        map[string]any{"depends_on": map[string]any{"app": dep("service_healthy")}},
		"seed":       map[string]any{"depends_on": map[string]any{"db": dep("service_healthy")}},
		"job":        map[string]any{"restart": "no"},
		"cache":      map[string]any{},
		"db":         map[string]any{"restart": "always"},
		"worker":     map[string]any{"restart": "unless-stopped"},
		"retry":      map[string]any{"restart": "on-failure:3"},
		"builder":    map[string]any{"profiles": []any{"build"}},
		"restart-no": map[string]any{"restart": "no", "depends_on": map[string]any{"cache": dep("service_started")}},
	}}
}

func TestOneShotsAreTheCompletedDependenciesAndTheServicesWithRestartNo(t *testing.T) {
	got := OneShots(oneShotConfig())
	table := []struct {
		service string
		oneShot bool
	}{
		{"seed", true},
		{"job", true},
		{"restart-no", true},
		{"app", false},
		{"web", false},
		{"cache", false},
		{"db", false},
		{"worker", false},
		{"retry", false},
		{"builder", false},
		{"gone", false},
	}
	if len(table) != 11 {
		t.Fatalf("%d rows, want 11", len(table))
	}
	for _, row := range table {
		if got[row.service] != row.oneShot {
			t.Errorf("%s: one-shot %v, want %v", row.service, got[row.service], row.oneShot)
		}
	}
	if want := map[string]bool{"seed": true, "job": true, "restart-no": true}; !maps.Equal(got, want) {
		t.Fatalf("one-shots %v, want %v", got, want)
	}
	if got := OneShots(nil); len(got) != 0 {
		t.Fatalf("no config gave one-shots %v", got)
	}
}

func TestResumeSkipsTheSameOneShotsThatReadinessReads(t *testing.T) {
	config := oneShotConfig()
	delete(config["services"].(map[string]any)["app"].(map[string]any)["depends_on"].(map[string]any), "gone")
	steps, err := ResidentOrder(config)
	if err != nil {
		t.Fatal(err)
	}
	residents := map[string][]string{}
	for _, s := range steps {
		residents[s.Service] = s.Healthy
	}
	oneShots := OneShots(config)
	for name := range config["services"].(map[string]any) {
		_, resident := residents[name]
		if resident == (oneShots[name] || name == "builder") {
			t.Errorf("%s: resident %v, one-shot %v", name, resident, oneShots[name])
		}
	}
	if len(residents) != 6 {
		t.Fatalf("residents %v, want app, web, cache, db, worker, retry", residents)
	}
	if healthy := residents["app"]; len(healthy) != 1 || healthy[0] != "db" {
		t.Fatalf("app waits for %v, want db through seed", healthy)
	}
}

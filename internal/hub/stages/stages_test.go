package stages

import (
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var stagesMutations = []porttest.Mutation{
	{"boot waits for running and healthy expected services and stops on an exited one", "treat every listed row as ready", "red: a starting or exited service counts as ready", "TestReadinessWaitsForHealthAndFailsOnAStoppedService"},
	{"an expected one-shot service that exited 0 counts as completed beside running services", "drop the one-shot exit 0 case so every exited row stops the boot", "red: the seed and job that exited 0 fail the boot", "TestReadinessReadsAOneShotThatExitedZeroAsCompletedAndEveryOtherExitAsAFailure"},
	{"a one-shot service that exited non-zero or died fails the boot with its name and code", "accept any exit of a one-shot service", "red: the seed that exited 1 reads as ready", "TestReadinessReadsAOneShotThatExitedZeroAsCompletedAndEveryOtherExitAsAFailure"},
	{"a long-running service exits as a boot failure even with code 0, whether its restart is unset or set", "read a service with unset restart as one-shot", "red: the app with no restart that exited 0 reads as ready", "TestReadinessReadsAOneShotThatExitedZeroAsCompletedAndEveryOtherExitAsAFailure"},
	{"stage limits match the documented values", "raise the suite limit to an hour", "red: the suite limit is not 20 minutes", "TestDefaultLimitsAreSixtyMinutesBuildTenBootTwentySuiteAndTwoForOneShotCalls"},
}

func TestStagesMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "stages_test.go", stagesMutations)
}

func TestDefaultLimitsAreSixtyMinutesBuildTenBootTwentySuiteAndTwoForOneShotCalls(t *testing.T) {
	l := DefaultLimits()
	if l.Build != 60*time.Minute || l.Boot != 10*time.Minute || l.Suite != 20*time.Minute || l.Call != 2*time.Minute || l.Poll <= 0 {
		t.Fatalf("limits %+v", l)
	}
}

func TestReadinessWaitsForHealthAndFailsOnAStoppedService(t *testing.T) {
	row := func(service, state, health string) compose.PsRow {
		return compose.PsRow{Service: service, State: state, Health: health}
	}
	expected := []string{"app", "db"}
	ready, _, err := Readiness([]compose.PsRow{row("app", "running", ""), row("db", "running", "healthy")}, expected, nil)
	if !ready || err != nil {
		t.Fatalf("ready services: %v %v", ready, err)
	}
	ready, waiting, err := Readiness([]compose.PsRow{row("app", "running", ""), row("db", "running", "starting")}, expected, nil)
	if ready || err != nil || !strings.Contains(waiting, "db") {
		t.Fatalf("starting db: %v %q %v", ready, waiting, err)
	}
	ready, waiting, _ = Readiness([]compose.PsRow{row("app", "running", "")}, expected, nil)
	if ready || !strings.Contains(waiting, "db (no container)") {
		t.Fatalf("missing db: %v %q", ready, waiting)
	}
	oneOff := compose.PsRow{Service: "db", State: "running", Labels: "com.docker.compose.oneoff=True"}
	if ready, _, _ := Readiness([]compose.PsRow{row("app", "running", ""), oneOff}, expected, nil); ready {
		t.Fatal("a one-off container counted as the db service")
	}
	if _, _, err := Readiness([]compose.PsRow{row("app", "exited", ""), row("db", "running", "healthy")}, expected, nil); err == nil {
		t.Fatal("an exited app did not fail the boot")
	}
}

func renderedServices() map[string]any {
	completed := map[string]any{"condition": "service_completed_successfully"}
	return map[string]any{"services": map[string]any{
		"app":    map[string]any{"image": "app", "depends_on": map[string]any{"seed": completed, "db": map[string]any{"condition": "service_healthy"}}},
		"db":     map[string]any{"image": "db", "restart": "always"},
		"seed":   map[string]any{"image": "seed"},
		"job":    map[string]any{"image": "job", "restart": "no"},
		"worker": map[string]any{"image": "worker", "restart": "unless-stopped"},
	}}
}

func TestReadinessReadsAOneShotThatExitedZeroAsCompletedAndEveryOtherExitAsAFailure(t *testing.T) {
	expected := []string{"app", "db", "seed", "job", "worker"}
	oneShot := compose.OneShots(renderedServices())
	exited := func(service string, code int) compose.PsRow {
		return compose.PsRow{Service: service, State: "exited", ExitCode: code}
	}
	running := func(service string) compose.PsRow { return compose.PsRow{Service: service, State: "running"} }
	healthyDB := compose.PsRow{Service: "db", State: "running", Health: "healthy"}
	cases := []struct {
		name    string
		rows    []compose.PsRow
		ready   bool
		waiting string
		err     string
	}{
		{"seed and job exited 0 beside running services", []compose.PsRow{running("app"), healthyDB, exited("seed", 0), exited("job", 0), running("worker")}, true, "", ""},
		{"seed and job still running", []compose.PsRow{running("app"), healthyDB, running("seed"), running("job"), running("worker")}, true, "", ""},
		{"seed created but not started", []compose.PsRow{running("app"), healthyDB, {Service: "seed", State: "created"}, exited("job", 0), running("worker")}, false, "seed (created )", ""},
		{"seed exited 1", []compose.PsRow{running("app"), healthyDB, exited("seed", 1), exited("job", 0), running("worker")}, false, "", "one-shot service seed failed (exited, exit code 1)"},
		{"job dead", []compose.PsRow{running("app"), healthyDB, exited("seed", 0), {Service: "job", State: "dead", ExitCode: 137}, running("worker")}, false, "", "one-shot service job failed (dead, exit code 137)"},
		{"app with no restart exited 0", []compose.PsRow{exited("app", 0), healthyDB, exited("seed", 0), exited("job", 0), running("worker")}, false, "", "app stopped (exited, exit code 0)"},
		{"unless-stopped worker exited 0", []compose.PsRow{running("app"), healthyDB, exited("seed", 0), exited("job", 0), exited("worker", 0)}, false, "", "worker stopped (exited, exit code 0)"},
		{"always db exited 0", []compose.PsRow{running("app"), exited("db", 0), exited("seed", 0), exited("job", 0), running("worker")}, false, "", "db stopped (exited, exit code 0)"},
	}
	if len(cases) != 8 {
		t.Fatalf("%d cases, want 8", len(cases))
	}
	for _, c := range cases {
		ready, waiting, err := Readiness(c.rows, expected, oneShot)
		got := ""
		if err != nil {
			got = err.Error()
		}
		if ready != c.ready || waiting != c.waiting || got != c.err {
			t.Errorf("%s: ready %v waiting %q error %q, want %v %q %q", c.name, ready, waiting, got, c.ready, c.waiting, c.err)
		}
	}
}

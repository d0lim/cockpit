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
	ready, _, err := Readiness([]compose.PsRow{row("app", "running", ""), row("db", "running", "healthy")}, expected)
	if !ready || err != nil {
		t.Fatalf("ready services: %v %v", ready, err)
	}
	ready, waiting, err := Readiness([]compose.PsRow{row("app", "running", ""), row("db", "running", "starting")}, expected)
	if ready || err != nil || !strings.Contains(waiting, "db") {
		t.Fatalf("starting db: %v %q %v", ready, waiting, err)
	}
	ready, waiting, _ = Readiness([]compose.PsRow{row("app", "running", "")}, expected)
	if ready || !strings.Contains(waiting, "db (no container)") {
		t.Fatalf("missing db: %v %q", ready, waiting)
	}
	oneOff := compose.PsRow{Service: "db", State: "running", Labels: "com.docker.compose.oneoff=True"}
	if ready, _, _ := Readiness([]compose.PsRow{row("app", "running", ""), oneOff}, expected); ready {
		t.Fatal("a one-off container counted as the db service")
	}
	if _, _, err := Readiness([]compose.PsRow{row("app", "exited", ""), row("db", "running", "healthy")}, expected); err == nil {
		t.Fatal("an exited app did not fail the boot")
	}
}

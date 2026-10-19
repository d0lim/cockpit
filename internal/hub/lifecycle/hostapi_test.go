package lifecycle

import (
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var hostAPIMutations = []porttest.Mutation{
	{"resume is withheld from a run pinned to a host API the hub does not serve", "ignore the served flag in AllowedEvents", "red: resume appears for an unserved run", "TestResumeIsAllowedOnlyForARunPinnedToAServedHostAPI"},
	{"served host API set membership", "Serves returns true", "red: host API 2 counts as served by a hub serving 1", "TestServesAcceptsOnlyHostAPIsInTheServedSet"},
	{"the refusal notice names the version, the served set and the hub version", "drop the served set from the notice", "red: the notice lacks the served set", "TestHostAPINoticeNamesTheUnknownVersionTheServedSetAndTheHubVersion"},
}

func TestResumeIsAllowedOnlyForARunPinnedToAServedHostAPI(t *testing.T) {
	stoppedRun := samples["done/stopped"]
	upRun := samples["done/up"]
	if got := AllowedEvents(stoppedRun, false); !slices.Equal(got, []Event{Destroy, Observed}) {
		t.Fatalf("unserved stopped run: %v", got)
	}
	if got := AllowedEvents(upRun, false); !slices.Equal(got, []Event{Stop, Destroy, Observed}) {
		t.Fatalf("unserved up run: %v", got)
	}
	if got := AllowedEvents(stoppedRun, true); !slices.Contains(got, Resume) {
		t.Fatalf("canary: served stopped run lacks resume: %v", got)
	}
}

func TestServesAcceptsOnlyHostAPIsInTheServedSet(t *testing.T) {
	served := []int{1}
	if !Serves(served, 1) {
		t.Fatal("canary: served version rejected")
	}
	for _, api := range []int{0, 2, -1} {
		if Serves(served, api) {
			t.Fatalf("host API %d counted as served", api)
		}
	}
}

func TestHostAPINoticeNamesTheUnknownVersionTheServedSetAndTheHubVersion(t *testing.T) {
	got := HostAPINotice(2, []int{1}, "v0.3.0")
	for _, want := range []string{"unknown host API version 2", "serves host API 1", "cockpit v0.3.0"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%q lacks %q", got, want)
		}
	}
}

func TestHostAPIMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "hostapi_test.go", hostAPIMutations)
}

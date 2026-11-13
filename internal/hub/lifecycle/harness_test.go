package lifecycle

import (
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var harnessNoticeMutations = []porttest.Mutation{
	{"the harness notice names the run's version, or 7 or earlier when none is recorded, and the hub's", "print the run version as a plain number", "red: a run without a recorded version reads harness version 0", "TestHarnessNoticeNamesTheRunsVersionOrSevenOrEarlierAndTheHubsVersion"},
}

func TestHarnessNoticeNamesTheRunsVersionOrSevenOrEarlierAndTheHubsVersion(t *testing.T) {
	cases := map[int]string{0: "made by harness version 7 or earlier,", 9: "made by harness version 9,"}
	checked := 0
	for run, want := range cases {
		got := HarnessNotice(run, 8)
		for _, part := range []string{want, "this hub runs harness version 8", "destroy the run and submit again"} {
			if !strings.Contains(got, part) {
				t.Errorf("run %d: %q lacks %q", run, got, part)
			}
		}
		checked++
	}
	if checked != 2 {
		t.Fatalf("checked %d cases", checked)
	}
}

func TestHarnessNoticeMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "harness_test.go", harnessNoticeMutations)
}

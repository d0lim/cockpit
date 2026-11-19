package api

import (
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/wire"
)

var wireLinkMutations = []porttest.Mutation{
	{"a root name is masked on each read", "return h.Link.Root unmasked from wireHead", "red: the value learned after the take is in the head", "TestARootNameLearnedAfterTheTakeIsMaskedInTheHeadAndTheReason"},
	{"the reason masks a root name", "drop the Root assignment in wireWhy", "red: the value is in why", "TestARootNameLearnedAfterTheTakeIsMaskedInTheHeadAndTheReason"},
}

func TestWireLinkMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "wirelinkmask_test.go", wireLinkMutations)
}

func TestARootNameLearnedAfterTheTakeIsMaskedInTheHeadAndTheReason(t *testing.T) {
	f, base := seededWire(t)
	tracedTake(t, f.runDir(rid(1)), "task "+planted)
	calls := getWire(t, f, base+"?take=traced&scope=outside").Calls
	if len(calls) != 1 || calls[0].Link != wire.LinkBackground {
		t.Fatalf("canary: the background call is listed: %+v", calls)
	}
	if root := calls[0].Root; strings.Contains(root, planted) || !strings.Contains(root, "****") {
		t.Fatalf("root %q", root)
	}
	call, _ := getWireCall(t, f, base+"/8?take=traced")
	if strings.Contains(call.Why, planted) || !strings.Contains(call.Why, "****") {
		t.Fatalf("why %q", call.Why)
	}
}

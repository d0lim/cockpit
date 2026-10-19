package scheduler

import (
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var ownerMutations = []porttest.Mutation{
	{"destroying another person's run needs force", "ownerDenied returns nil", "red: cli destroys alice's run without force", "TestDestroyingAnotherSubmittersRunNeedsForce"},
	{"stop and unkeep follow the same rule", "drop the guard from Stop and setKeep", "red: cli stops and unkeeps alice's runs", "TestStopAndUnkeepOfAnotherSubmittersRunNeedForceAndTheOwnerPasses"},
}

func alice(m *store.Meta) { m.Submitter = "alice" }

func TestDestroyingAnotherSubmittersRunNeedsForce(t *testing.T) {
	s, w := setup(t, []store.Meta{preserved(1, 10, alice)})
	r := s.Destroy(rid(1), Actor{By: "cli"})
	if r.Fail == nil || r.Fail.Status != 409 || r.Fail.Code != ticker.CodeConflictOwner || read(t, w, rid(1)).Env != lifecycle.EnvUp {
		t.Fatalf("%+v", r)
	}
	if r := s.Destroy(rid(1), Actor{By: "cli", Force: true}); !r.OK() || read(t, w, rid(1)).Env != lifecycle.EnvDestroyed {
		t.Fatalf("%+v", r.Fail)
	}
}

func TestStopAndUnkeepOfAnotherSubmittersRunNeedForceAndTheOwnerPasses(t *testing.T) {
	s, w := setup(t, []store.Meta{preserved(1, 10, alice, kept), preserved(2, 11, alice)})
	if r := s.Unkeep(rid(1), Actor{By: "cli"}); r.Fail == nil || r.Fail.Code != ticker.CodeConflictOwner {
		t.Fatalf("%+v", r)
	}
	if r := s.Stop(rid(2), Actor{By: "cli"}); r.Fail == nil || r.Fail.Code != ticker.CodeConflictOwner {
		t.Fatalf("%+v", r)
	}
	if r := s.Unkeep(rid(1), Actor{By: "alice"}); !r.OK() {
		t.Fatal(r.Fail)
	}
	if r := s.Stop(rid(2), Actor{By: "cli", Force: true}); !r.OK() {
		t.Fatal(r.Fail)
	}
	if read(t, w, rid(1)).Keep || read(t, w, rid(2)).Env != lifecycle.EnvStopped || read(t, w, rid(2)).StopReason != lifecycle.StopPerson {
		t.Fatalf("%+v %+v", read(t, w, rid(1)), read(t, w, rid(2)))
	}
}

func TestOwnerMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "owner_test.go", ownerMutations)
}

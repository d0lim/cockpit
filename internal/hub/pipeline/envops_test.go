package pipeline

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var _ ticker.Env = (*envops.Ops)(nil)

var envopsMutations = []porttest.Mutation{
	{"resume checks the pinned runner image before starting", "skip VerifyPin in Resume", "red: no tag repair precedes docker start", "TestResumeRepairsThePinnedImageStartsContainersAndRefusesTakenPorts"},
	{"resume refuses taken ports without leasing again", "skip the BlockedPorts check in Resume", "red: a resume with a taken port starts containers", "TestResumeRepairsThePinnedImageStartsContainersAndRefusesTakenPorts"},
	{"destroy removes the environment directories and the lease but keeps meta", "skip removeEnvDirs or releaseLease", "red: src stays or the lease remains after destroy", "TestStopThenDestroyLeavesOnlyTheRecordAndReleasesTheLease"},
}

func TestEnvopsMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "envops_test.go", envopsMutations)
}

func TestResumeRepairsThePinnedImageStartsContainersAndRefusesTakenPorts(t *testing.T) {
	w := newWorld(t, nil, nil)
	m := w.submit()
	if res := w.sched.Stop(m.ID, scheduler.Actor{By: "tester"}); res.Fail != nil {
		t.Fatal(res.Fail)
	}
	w.fake.TagID = fakedocker.OtherID
	calls := len(w.fake.Calls())
	res := w.sched.Resume(m.ID, scheduler.Actor{By: "tester"})
	if res.Fail != nil || res.Run.Env != lifecycle.EnvUp {
		t.Fatalf("resume: %+v %+v", res.Fail, res.Run.State)
	}
	var verbs []string
	for _, c := range w.fake.Calls()[calls:] {
		verbs = append(verbs, fakedocker.Verb(c.Args))
	}
	tag, start := indexOf(verbs, "tag"), indexOf(verbs, "start")
	if tag < 0 || start < tag {
		t.Fatalf("resume calls %v", verbs)
	}
	if res := w.sched.Stop(m.ID, scheduler.Actor{By: "tester"}); res.Fail != nil {
		t.Fatal(res.Fail)
	}
	taken := w.base + 1
	w.hub.ops.Probe = func(port int) bool { return port != taken }
	calls = len(w.fake.Calls())
	res = w.sched.Resume(m.ID, scheduler.Actor{By: "tester"})
	if res.Fail == nil || res.Fail.Code != ticker.CodeConflictPort || !strings.Contains(res.Fail.Reason, strconv.Itoa(taken)) {
		t.Fatalf("taken port: %+v", res.Fail)
	}
	for _, c := range w.fake.Calls()[calls:] {
		if c.Args[0] == "start" {
			t.Fatal("containers started despite a taken port")
		}
	}
	if after := w.meta(m.ID); after.Phase != lifecycle.Done || after.Env != lifecycle.EnvStopped {
		t.Fatalf("after refusal %+v", after.State)
	}
}

func TestStopThenDestroyLeavesOnlyTheRecordAndReleasesTheLease(t *testing.T) {
	w := newWorld(t, nil, nil)
	m := w.submit()
	if res := w.sched.Stop(m.ID, scheduler.Actor{By: "tester"}); res.Fail != nil || res.Run.Env != lifecycle.EnvStopped {
		t.Fatalf("stop: %+v", res)
	}
	if res := w.sched.Destroy(m.ID, scheduler.Actor{By: "tester"}); res.Fail != nil || res.Run.Env != lifecycle.EnvDestroyed {
		t.Fatalf("destroy: %+v", res)
	}
	dir, _ := w.hub.Store().RunDir(m.ID)
	for _, name := range envops.EnvDirs {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s survived: %v", name, err)
		}
	}
	for _, name := range []string{"meta.json", "seal.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s is gone: %v", name, err)
		}
	}
	st, _ := w.hub.Store().CurrentState()
	if _, ok := st.Leases[m.ID]; ok {
		t.Fatal("the lease survived destroy")
	}
	if indexOf(w.verbs(), "compose down") < 0 {
		t.Fatal("destroy did not bring the project down")
	}
}

package pipeline

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/collect/collecttest"
	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const hangingCollector = `import { type CollectCtx, collector } from "cockpit:collect";

export default collector({
	id: "hang",
	intervalMs: 1000,
	async run(ctx: CollectCtx) {
		while (true) {}
	},
});
`

var childKillMutations = []porttest.Mutation{
	{"a dead collector child is an unread record", "drop the ledger.add of the failure reason in run.record", "red: no unread record names the killed child", "TestKillingACollectorChildDuringTestingLeavesAnUnreadRecordAndTheRunStillPasses"},
	{"a dead collector child never changes the verdict", "set the meta verdict to error in DefaultReport when collector unread records exist", "red: the run closes as error instead of passed", "TestKillingACollectorChildDuringTestingLeavesAnUnreadRecordAndTheRunStillPasses"},
}

func TestChildKillMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "childkill_test.go", childKillMutations)
}

func scriptHostChildren(t *testing.T) []int {
	t.Helper()
	out, _ := exec.Command("pgrep", "-P", strconv.Itoa(os.Getpid()), "-f", worker.SubCommand).Output()
	var pids []int
	for _, field := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(field); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

func TestKillingACollectorChildDuringTestingLeavesAnUnreadRecordAndTheRunStillPasses(t *testing.T) {
	w := newWorldWith(t, func(o *Options) {
		o.NoCollection, o.ScriptHost, o.Limits.Suite = false, collecttest.Command(), 20*time.Second
		o.Collect = collect.Options{DrainWait: 2 * time.Second, Heartbeat: 100 * time.Millisecond, MaxBudget: 15 * time.Second, ReportBudget: time.Second}
	}, func(f *fakedocker.Fake) {
		f.RunnerDelay = 4 * time.Second
	}, func(dir string) {
		os.RemoveAll(filepath.Join(dir, "scripts", "collectors"))
		writeRepoFile(t, filepath.Join(dir, "scripts", "collectors", "hang.ts"), hangingCollector)
	})
	res := w.sched.Submit(context.Background(), scheduler.SubmitInput{Worktree: w.repo, Submitter: "tester"})
	if res.Fail != nil {
		t.Fatal(res.Fail)
	}
	finished := make(chan struct{})
	go func() {
		w.sched.Tick()
		w.sched.Idle()
		close(finished)
	}()
	var victim int
	waitFor(t, "a script-host child while the run is testing", func() bool {
		pids := scriptHostChildren(t)
		if len(pids) == 0 || w.meta(res.ID).Phase != lifecycle.Testing {
			return false
		}
		victim = pids[0]
		return true
	})
	if err := syscall.Kill(-victim, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "an unread record for the killed child", func() bool {
		for _, u := range w.hub.ops.Collect.Unread(res.ID) {
			if u.Collector == "hang" && u.Source == "collector:hang" && strings.Contains(u.Reason, "exited without a result") {
				return true
			}
		}
		return false
	})
	if m := w.meta(res.ID); m.Phase != lifecycle.Testing || m.Verdict != lifecycle.VerdictNone {
		t.Fatalf("the kill moved the run to %+v", m.State)
	}
	select {
	case <-finished:
	case <-time.After(30 * time.Second):
		t.Fatal("the run did not finish")
	}
	if m := w.meta(res.ID); m.Phase != lifecycle.Done || m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("state %+v failed stage %q notice %q", m.State, m.FailedStage, m.Notice)
	}
}

package pipeline

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/collect/collecttest"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var reportBudgetMutations = []porttest.Mutation{
	{"a collector that hangs at report time is cut at the report budget and the bundle is still written", "run report-time collectors under the caller context with no budget timer", "red: the report waits for the 10s exec limit and the run takes over 9s", "TestACollectorHangingAtReportTimeIsCutAtTheBudgetAndTheBundleStillCarriesAnUnreadRecord"},
	{"the report-time budget failure is an unread record", "drop the fold of the budget reason in Supervisor.Report", "red: no unread record names the report budget", "TestACollectorHangingAtReportTimeIsCutAtTheBudgetAndTheBundleStillCarriesAnUnreadRecord"},
	{"a report the hub makes on reconciliation with the environment stopped skips the collectors", "run the report-time collectors whatever the environment", "red: the beat stage is ok or error instead of skipped", "TestAReconciledReportWithTheEnvironmentStoppedSkipsTheCollectors"},
}

func TestReportBudgetMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "reportbudget_test.go", reportBudgetMutations)
}

func TestACollectorHangingAtReportTimeIsCutAtTheBudgetAndTheBundleStillCarriesAnUnreadRecord(t *testing.T) {
	w := newWorldWith(t, func(o *Options) {
		o.NoCollection, o.ScriptHost, o.Limits.Suite = false, collecttest.Command(), 10*time.Second
		o.Collect = collect.Options{DrainWait: 2 * time.Second, Heartbeat: 100 * time.Millisecond, ReportBudget: time.Second}
	}, func(f *fakedocker.Fake) {
		f.RunnerDelay, f.ExecHangsLate = 1500*time.Millisecond, true
	}, func(dir string) {
		os.RemoveAll(filepath.Join(dir, "scripts", "collectors"))
		writeRepoFile(t, filepath.Join(dir, "scripts", "collectors", "dbsize.ts"), execCollector)
	})
	started := time.Now()
	m := w.submit()
	if m.Verdict != lifecycle.VerdictPassed || time.Since(started) > 9*time.Second {
		t.Fatalf("state %+v notice %q after %s", m.State, m.Notice, time.Since(started))
	}
	dir, _ := w.hub.Store().RunDir(m.ID)
	doc, _ := readSummary(t, dir)
	stages, _ := doc["stages"].(map[string]any)
	stage, _ := stages["dbsize"].(map[string]any)
	if stage["status"] != "error" || !strings.Contains(stage["detail"].(string), "report budget of 1s") {
		t.Fatalf("dbsize stage %v", stages["dbsize"])
	}
	unread := doc["notProven"].(map[string]any)["unread"].([]any)
	budget := 0
	for _, line := range unread {
		if strings.Contains(line.(string), "dbsize") && strings.Contains(line.(string), "report budget of 1s") {
			budget++
		}
	}
	if budget == 0 {
		t.Fatalf("no unread record names the report budget: %v", unread)
	}
}

func TestAReconciledReportWithTheEnvironmentStoppedSkipsTheCollectors(t *testing.T) {
	home, crashed := t.TempDir(), filepath.Join(t.TempDir(), "crashed")
	collecting := func(o *Options) {
		o.NoCollection, o.ScriptHost, o.Limits.Suite = false, collecttest.Command(), time.Minute
		o.Collect = collect.Options{DrainWait: 2 * time.Second, Heartbeat: 100 * time.Millisecond}
	}
	beat := func(dir string) {
		os.RemoveAll(filepath.Join(dir, "scripts", "collectors"))
		writeRepoFile(t, filepath.Join(dir, "scripts", "collectors", "beat.ts"), beatCollector)
	}
	first := newWorldWith(t, func(o *Options) { collecting(o); o.Home = home }, func(f *fakedocker.Fake) { f.RunnerHangs = true }, beat)
	res := first.sched.Submit(context.Background(), scheduler.SubmitInput{Worktree: first.repo, Submitter: "tester"})
	if res.Fail != nil {
		t.Fatal(res.Fail)
	}
	waitFor(t, "the run to reach testing with a beat signal", func() bool {
		dir, _ := first.hub.Store().RunDir(res.ID)
		data, _ := os.ReadFile(filepath.Join(dir, "signals", "stream.jsonl"))
		return first.meta(res.ID).Phase == lifecycle.Testing && strings.Contains(string(data), `"collector":"beat"`)
	})
	copyLiveHome(t, home, crashed)
	first.sched.Dispose()
	first.sched.Idle()
	first.hub.Close()
	second := newWorldWith(t, func(o *Options) { collecting(o); o.Home = crashed }, nil, beat)
	if m := second.meta(res.ID); m.Phase != lifecycle.Testing {
		t.Fatalf("the copied home holds the run in %s", m.Phase)
	}
	second.sched.Reconcile()
	m := second.meta(res.ID)
	if m.Phase != lifecycle.Done || m.Env != lifecycle.EnvStopped || m.Verdict != lifecycle.VerdictError {
		t.Fatalf("reconciled state %+v notice %q", m.State, m.Notice)
	}
	dir, _ := second.hub.Store().RunDir(res.ID)
	doc, _ := readSummary(t, dir)
	stages, _ := doc["stages"].(map[string]any)
	if stage, _ := stages["beat"].(map[string]any); stage["status"] != "skipped" || stage["detail"] != "the environment is stopped" {
		t.Fatalf("beat stage %v", stages["beat"])
	}
}

func copyLiveHome(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) || strings.HasSuffix(path, ".tmp") {
			return nil
		}
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		target := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

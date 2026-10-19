package pipeline

import (
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
	"github.com/d0lim/cockpit/internal/report"
)

var collectMutations = []porttest.Mutation{
	{"collectors arm on boot and their signals reach the bundle", "skip the arm hook in Ops.Transition", "red: no beat metric before the report and no observer signal in the bundle", "TestArmedCollectorsAndObserversReachTheBundleAndStopResumeLeavesNoFailures"},
	{"stop drains collectors before compose stop", "turn drain hooks into no-ops in Ops.Transition", "red: beat keeps emitting while the environment is stopped", "TestArmedCollectorsAndObserversReachTheBundleAndStopResumeLeavesNoFailures"},
	{"the report stage feeds collector stages into the summary", "drop Collected from the report input", "red: summary stages have no beat entry", "TestArmedCollectorsAndObserversReachTheBundleAndStopResumeLeavesNoFailures"},
	{"container log lines are masked before they reach the bundle", "pass ports.Identity to the logs observer and open the stream with an empty masker (either alone stays green: the other layer masks)", "red: the planted container value stays raw and the bundle is discarded", "TestArmedCollectorsAndObserversReachTheBundleAndStopResumeLeavesNoFailures"},
	{"a kept run that stays up after the report keeps collecting", "drop the reported-with-env-up case from lifecycle.HookFor", "red: no beat is appended after the report and the collector state is stopped", "TestAKeptRunThatStaysUpAfterTheReportKeepsCollecting"},
}

func TestCollectMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "collect_test.go", collectMutations)
}

const beatCollector = `import { type CollectCtx, collector } from "cockpit:collect";

export default collector({
	id: "beat",
	intervalMs: 1000,
	async run(ctx: CollectCtx) {
		ctx.signal.metric({ name: "beat.count", value: 1 });
	},
});
`

func collectingWorld(t *testing.T) *world {
	return newWorldWith(t, func(o *Options) {
		o.NoCollection, o.ScriptHost, o.Limits.Suite = false, collecttest.Command(), 10*time.Second
		o.Collect = collect.Options{DrainWait: 2 * time.Second, Heartbeat: 100 * time.Millisecond}
	}, func(f *fakedocker.Fake) {
		f.RunnerDelay, f.ContainerSecret = 2500*time.Millisecond, planted
	}, func(dir string) {
		os.RemoveAll(filepath.Join(dir, "scripts", "collectors"))
		writeRepoFile(t, filepath.Join(dir, "scripts", "collectors", "beat.ts"), beatCollector)
	})
}

func writeRepoFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertNoCollectorFailures(t *testing.T, h *Hub, id, when string) {
	t.Helper()
	sup := h.ops.Collect
	if unread := sup.Unread(id); len(unread) != 0 {
		t.Errorf("%s: unread %+v", when, unread)
	}
	statuses := sup.Status(id)
	for _, s := range statuses {
		if s.Failures != 0 {
			t.Errorf("%s: %s failed %d times: %s", when, s.ID, s.Failures, s.Reason)
		}
	}
	if len(statuses) == 0 {
		t.Errorf("%s: no collector status", when)
	}
}

func TestArmedCollectorsAndObserversReachTheBundleAndStopResumeLeavesNoFailures(t *testing.T) {
	w := collectingWorld(t)
	m := w.submit()
	if m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	dir, _ := w.hub.Store().RunDir(m.ID)
	frozen := fileText(t, filepath.Join(dir, report.DirReport, "signals", "stream.jsonl"))
	for _, want := range []string{`"collector":"beat"`, `"collector":"services"`, `"collector":"logs"`, fakedocker.ContainerLogLine} {
		if !strings.Contains(frozen, want) {
			t.Errorf("the bundle stream lacks %s", want)
		}
	}
	if strings.Contains(frozen, planted) || !strings.Contains(frozen, "****") {
		t.Error("the bundle stream holds the planted container value or masked nothing")
	}
	doc, _ := readSummary(t, dir)
	stages, _ := doc["stages"].(map[string]any)
	if beat, _ := stages["beat"].(map[string]any); beat["status"] != "ok" {
		t.Errorf("beat stage %v", stages["beat"])
	}
	assertNoCollectorFailures(t, w.hub, m.ID, "after the report")
	actor := scheduler.Actor{By: "tester"}
	if r := w.sched.Stop(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	if r := w.sched.Resume(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	time.Sleep(1500 * time.Millisecond)
	if r := w.sched.Stop(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	stopped := strings.Count(fileText(t, filepath.Join(dir, "signals", "stream.jsonl")), `"collector":"beat"`)
	time.Sleep(1500 * time.Millisecond)
	if still := strings.Count(fileText(t, filepath.Join(dir, "signals", "stream.jsonl")), `"collector":"beat"`); still != stopped {
		t.Errorf("beat emitted while the environment was stopped (%d -> %d)", stopped, still)
	}
	if r := w.sched.Resume(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	before := strings.Count(fileText(t, filepath.Join(dir, "signals", "stream.jsonl")), `"collector":"beat"`)
	time.Sleep(2500 * time.Millisecond)
	if r := w.sched.Stop(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	after := strings.Count(fileText(t, filepath.Join(dir, "signals", "stream.jsonl")), `"collector":"beat"`)
	if after <= before {
		t.Errorf("beat did not run after resume (%d -> %d)", before, after)
	}
	assertNoCollectorFailures(t, w.hub, m.ID, "around stop and resume")
}

func fileText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAKeptRunThatStaysUpAfterTheReportKeepsCollecting(t *testing.T) {
	w := collectingWorld(t)
	m := w.submit()
	if m.Verdict != lifecycle.VerdictPassed || m.Env != lifecycle.EnvUp {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	dir, _ := w.hub.Store().RunDir(m.ID)
	live := filepath.Join(dir, "signals", "stream.jsonl")
	reported := strings.Count(fileText(t, live), `"collector":"beat"`)
	time.Sleep(2500 * time.Millisecond)
	later := strings.Count(fileText(t, live), `"collector":"beat"`)
	if later <= reported {
		t.Fatalf("no beat after the report on a kept run (%d -> %d)", reported, later)
	}
	states := w.hub.ops.Collect.Status(m.ID)
	if len(states) != 1 || states[0].State != collect.StateArmed {
		t.Fatalf("collector states %+v", states)
	}
	assertNoCollectorFailures(t, w.hub, m.ID, "after the report on a kept run")
	if r := w.sched.Stop(m.ID, scheduler.Actor{By: "tester"}); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	if states := w.hub.ops.Collect.Status(m.ID); states[0].State != collect.StateStopped {
		t.Fatalf("after stop %+v", states)
	}
}

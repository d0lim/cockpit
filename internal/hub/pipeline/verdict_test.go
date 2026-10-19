package pipeline

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var verdictMutations = []porttest.Mutation{
	{"OOM, the time limit and exit codes outside 0 and 1 close as error", "skip the runner checks in DefaultReport", "red: an OOM-killed or exit-3 runner passes", "TestRunnerOOMTimeLimitAndExitCodesOutsideZeroAndOneCloseAsErrorAndExitOneFails"},
	{"a runner identity that differs from the pin closes as error with both values", "skip IdentityMismatch", "red: a foreign runner image passes", "TestRunnerIdentityDifferentFromThePinClosesAsErrorNamingBothValues"},
	{"a runner version differs only when both sides name a revision or a release", "make SameVersion return true", "red: runner 1.0.1 under hub 1.0.0 passes", "TestRunnerIdentityDifferentFromThePinClosesAsErrorNamingBothValues"},
	{"the tag is checked against the pinned ID before the runner runs", "skip EnsurePinned in runSuite", "red: no retag or re-import before compose run runner", "TestTagPointingAwayFromThePinnedIDIsRepairedBeforeTheRunnerRuns"},
	{"runner lines pass the run redactor before events and meta tail", "skip the mask in live.say and in Project.masked", "red: the planted token appears in the events or the tail", "TestPlantedSecretInRunnerLogsNeverReachesEventsOrTheMetaTailInPlaintext"},
	{"lines that are only a declared secret file name stay out of the tail", "drop the CredentialPathOnly filter in tail", "red: config/app.secrets appears in the tail", "TestPlantedSecretInRunnerLogsNeverReachesEventsOrTheMetaTailInPlaintext"},
	{"the pgid is recorded while docker runs and cleared after", "build the run docker without PgidHooks in begin and ensureLoaded", "red: no pgid is observed during the run", "TestProcessGroupIsRecordedInMetaWhileDockerRunsAndClearedAfterTheRun"},
	{"an abort during the runner closes as aborted and force-removes the container", "treat an aborted runner as a stage failure", "red: the verdict is error or no rm -f follows", "TestAbortWhileTheRunnerRunsClosesAsAbortedAndForceRemovesTheRunner"},
}

func TestVerdictMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "verdict_test.go", verdictMutations)
}

func TestRunnerOOMTimeLimitAndExitCodesOutsideZeroAndOneCloseAsErrorAndExitOneFails(t *testing.T) {
	failedSteps := []contract.ResultStep{{Flow: "health", ID: "health-ok", Status: contract.StepStatusFailed}}
	cases := []struct {
		name    string
		mutate  func(*fakedocker.Fake)
		verdict lifecycle.Verdict
		says    string
	}{
		{"oom", func(f *fakedocker.Fake) { f.RunnerOOM, f.RunnerCode = true, 137 }, lifecycle.VerdictError, "out of memory"},
		{"time limit", func(f *fakedocker.Fake) { f.RunnerHangs = true }, lifecycle.VerdictError, "limit"},
		{"exit code 3", func(f *fakedocker.Fake) { f.RunnerCode = 3 }, lifecycle.VerdictError, "outside 0 and 1"},
		{"exit code 1", func(f *fakedocker.Fake) { f.RunnerCode, f.Steps = 1, failedSteps }, lifecycle.VerdictFailed, "failed"},
	}
	for _, c := range cases {
		m := newWorld(t, c.mutate, nil).submit()
		if m.Phase != lifecycle.Done || m.Verdict != c.verdict || !strings.Contains(m.Notice, c.says) {
			t.Errorf("%s: state %+v notice %q", c.name, m.State, m.Notice)
		}
	}
}

func TestRunnerIdentityDifferentFromThePinClosesAsErrorNamingBothValues(t *testing.T) {
	foreign := "cockpit-runner:ffffffffffff"
	m := newWorld(t, func(f *fakedocker.Fake) { f.ManifestImage = foreign }, nil).submit()
	if m.Verdict != lifecycle.VerdictError || !strings.Contains(m.Notice, foreign) || !strings.Contains(m.Notice, fakedocker.RunnerTag) {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	pin := &contract.RunnerImage{Tag: fakedocker.RunnerTag}
	versions := []struct {
		runner, hub string
		same        bool
	}{
		{"1.0.0", "1.0.0", true},
		{"dev+5aef27003df4", "dev", true},
		{"dev", "dev+5aef27003df4", true},
		{"1.0.1", "1.0.0", false},
		{"dev+5aef27003df4", "dev+82bec7d00000", false},
		{"dev+5aef27003df4", "1.0.0", false},
	}
	for _, v := range versions {
		got := IdentityMismatch(&contract.Manifest{Runner: &contract.RunnerIdentity{Image: fakedocker.RunnerTag, Version: v.runner}}, pin, v.hub)
		if (got == "") != v.same || !v.same && (!strings.Contains(got, v.runner) || !strings.Contains(got, v.hub)) {
			t.Errorf("runner %s hub %s: %q", v.runner, v.hub, got)
		}
	}
	if len(versions) != 6 {
		t.Fatal(len(versions))
	}
}

func runnerStartAfter(verbs []string, before string) bool {
	run := indexOf(verbs, "compose run runner")
	at := slices.Index(verbs, before)
	return at >= 0 && run > at
}

func TestTagPointingAwayFromThePinnedIDIsRepairedBeforeTheRunnerRuns(t *testing.T) {
	w := newWorld(t, func(f *fakedocker.Fake) { f.TagID = fakedocker.OtherID }, nil)
	m := w.submit()
	if m.Verdict != lifecycle.VerdictPassed || !runnerStartAfter(w.verbs(), "tag") {
		t.Fatalf("retag: verdict %s verbs %v", m.Verdict, w.verbs())
	}
	for _, c := range w.fake.Calls() {
		if c.Args[0] == "tag" && (c.Args[1] != fakedocker.PinnedID || c.Args[2] != fakedocker.RunnerTag) {
			t.Fatalf("tag call %q", c.Args)
		}
	}
	gone := newWorld(t, func(f *fakedocker.Fake) { f.TagID, f.PinnedGone = fakedocker.OtherID, true }, nil)
	m = gone.submit()
	if gone.ensures != 2 || indexOf(gone.verbs(), "tag") >= 0 || m.RunnerImage.Tag != fakedocker.RunnerTag {
		t.Fatalf("re-import: ensures %d verbs %v pin %+v", gone.ensures, gone.verbs(), m.RunnerImage)
	}
	matched := newWorld(t, nil, nil)
	matched.submit()
	if matched.ensures != 1 || indexOf(matched.verbs(), "tag") >= 0 {
		t.Fatalf("canary: a matching tag was repaired (ensures %d)", matched.ensures)
	}
}

func TestPlantedSecretInRunnerLogsNeverReachesEventsOrTheMetaTailInPlaintext(t *testing.T) {
	w := newWorld(t, func(f *fakedocker.Fake) {
		f.RunnerHangs = true
		f.RunnerLogs += "config/app.secrets\nplain progress line\n"
		f.EnvFiles["config/app.secrets"] = "EXTRA=extra-secret-value-123\n"
	}, func(dir string) {
		decl := filepath.Join(dir, ".cockpit", "project.yaml")
		replaceIn(t, decl, "outputs: [.env, e2e.private.env.json]", "outputs: [.env, e2e.private.env.json, config/app.secrets]")
	})
	m := w.submit()
	tail := strings.Join(m.Tail, "\n")
	for name, text := range map[string]string{"events": w.eventText(), "tail": tail} {
		if strings.Contains(text, planted) || !strings.Contains(text, "****") {
			t.Errorf("%s:\n%s", name, text)
		}
	}
	if strings.Contains(tail, "config/app.secrets") || !strings.Contains(tail, "plain progress line") {
		t.Fatalf("tail filter:\n%s", tail)
	}
}

func TestProcessGroupIsRecordedInMetaWhileDockerRunsAndClearedAfterTheRun(t *testing.T) {
	var w *world
	seen := 0
	w = newWorld(t, func(f *fakedocker.Fake) {
		f.OnSpawn = func(pgid int) {
			if w == nil {
				return
			}
			list, _ := w.hub.Store().ListRuns()
			for _, m := range list.Runs {
				if m.Pgid != nil && *m.Pgid == fakedocker.FakePgid {
					seen++
				}
			}
		}
	}, nil)
	m := w.submit()
	if seen == 0 || m.Pgid != nil || m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("pgid seen %d times, final %v, verdict %s", seen, m.Pgid, m.Verdict)
	}
}

func TestAbortWhileTheRunnerRunsClosesAsAbortedAndForceRemovesTheRunner(t *testing.T) {
	w := newWorld(t, func(f *fakedocker.Fake) { f.RunnerHangs = true }, nil)
	w.hub.o.Limits.Suite = time.Minute
	w.hub.ops.Limits.Suite = time.Minute
	res := w.sched.Submit(t.Context(), scheduler.SubmitInput{Worktree: w.repo, Submitter: "tester"})
	if res.Fail != nil {
		t.Fatal(res.Fail)
	}
	w.sched.Tick()
	deadline := time.Now().Add(5 * time.Second)
	for indexOf(w.verbs(), "wait") < 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if r := w.sched.Abort(res.ID, scheduler.Actor{By: "tester"}); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	w.sched.Idle()
	m := w.meta(res.ID)
	if m.Phase != lifecycle.Done || m.Verdict != lifecycle.VerdictAborted || m.FailedStage != lifecycle.Testing {
		t.Fatalf("state %+v failed stage %s notice %q", m.State, m.FailedStage, m.Notice)
	}
	forced := false
	for _, c := range w.fake.Calls() {
		if slices.Equal(c.Args, []string{"rm", "-f", fakedocker.ContainerID}) {
			forced = true
		}
	}
	if !forced {
		t.Fatalf("the runner was not force-removed: %v", w.verbs())
	}
}

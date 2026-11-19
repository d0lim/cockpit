package pipeline

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/wire"
)

func TestTheWireRecordLivesUnderTheBundleDirectoryTheReportWrites(t *testing.T) {
	if wire.BundleDir != report.DirReport {
		t.Fatalf("wire.BundleDir %q, report.DirReport %q", wire.BundleDir, report.DirReport)
	}
}

func TestATimedOutRunStillGetsItsRecord(t *testing.T) {
	w := wireWorld(t, func(f *fakedocker.Fake) { f.RunnerHangs = true })
	m, dir := w.reportedWire()
	if m.Verdict != lifecycle.VerdictError || !strings.Contains(m.Notice, "limit") {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	if status := summaryWire(t, dir); status.State != wire.StateTaken || status.Calls != 2 {
		t.Fatalf("summary.wire %+v", status)
	}
	present(t, filepath.Join(bundleRecord(dir), wire.SuiteTake, wire.CallsFile))
}

func TestAnAbortedRunStillGetsItsRecord(t *testing.T) {
	w := wireWorld(t, func(f *fakedocker.Fake) { f.RunnerHangs = true })
	w.hub.o.Limits.Suite, w.hub.ops.Limits.Suite = time.Minute, time.Minute
	res := w.sched.Submit(t.Context(), scheduler.SubmitInput{Worktree: w.repo, Submitter: "tester", Wire: true})
	if res.Fail != nil {
		t.Fatal(res.Fail)
	}
	w.sched.Tick()
	waitFor(t, "the runner wait", func() bool { return indexOf(w.verbs(), "wait") >= 0 })
	if r := w.sched.Abort(res.ID, tester); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	w.sched.Idle()
	m := w.meta(res.ID)
	if m.Phase != lifecycle.Done || m.Verdict != lifecycle.VerdictAborted {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	dir, _ := w.hub.Store().RunDir(res.ID)
	if status := summaryWire(t, dir); status.State != wire.StateTaken || status.Calls != 2 {
		t.Fatalf("summary.wire %+v", status)
	}
}

func TestATakeThatCannotCompleteLeavesTheBundleAndOneUnreadLine(t *testing.T) {
	cases := map[string]struct {
		edit  func(*fakedocker.Fake)
		state wire.State
	}{
		"a recorder that is gone": {func(f *fakedocker.Fake) { f.Wire.Gone = true }, wire.StateLost},
		"a stream broken twice":   {func(f *fakedocker.Fake) { f.Wire.Broken = 2 }, wire.StateFailed},
		"a recorder restarted during the suite": {func(f *fakedocker.Fake) {
			f.Wire.Since = time.Date(2026, 10, 19, 1, 0, 5, 0, time.UTC)
		}, wire.StateLost},
	}
	executed := 0
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			w := wireWorld(t, c.edit)
			m, dir := w.reportedWire()
			if m.Verdict != lifecycle.VerdictPassed || m.Redaction != nil {
				t.Fatalf("the take changed the run: %+v notice %q redaction %+v", m.State, m.Notice, m.Redaction)
			}
			assertBundle(t, dir)
			if status := summaryWire(t, dir); status.State != c.state || status.Calls != 0 || status.Detail == "" {
				t.Fatalf("summary.wire %+v", status)
			}
			oneUnreadWireLine(t, dir, c.state)
			absent(t, filepath.Join(bundleRecord(dir), wire.SuiteTake))
			absent(t, stagedRecord(dir))
			text, err := os.ReadFile(filepath.Join(dir, report.DirReport, report.FileNotProven))
			if err != nil || !strings.Contains(string(text), stageWire) {
				t.Fatalf("not-proven.md %v:\n%s", err, text)
			}
		})
		executed++
	}
	if executed != len(cases) || executed != 3 {
		t.Fatalf("executed %d cases", executed)
	}
}

func TestAStreamBrokenOnceIsTakenAgainAndARecorderStartedBeforeTheStepsKeepsItsRecord(t *testing.T) {
	w := wireWorld(t, func(f *fakedocker.Fake) {
		f.Wire.Broken = 1
		f.Wire.Since = time.Date(2026, 10, 19, 1, 0, 1, 0, time.UTC)
	})
	_, dir := w.reportedWire()
	if status := summaryWire(t, dir); status.State != wire.StateTaken || status.Calls != 2 {
		t.Fatalf("summary.wire %+v", status)
	}
	if taken := w.fake.Wire.Taken(); len(taken) != 2 {
		t.Fatalf("takes %v, want one retry", taken)
	}
}

func TestASecondReportKeepsTheRecordAndDoesNotAskTheRecorderAgain(t *testing.T) {
	w := wireWorld(t, nil)
	m, dir := w.reportedWire()
	before := wireFiles(t, bundleRecord(dir))
	asked := len(w.fake.Wire.Taken())
	w.fake.Wire.Gone = true
	if _, err := w.hub.ops.Report(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	after := wireFiles(t, bundleRecord(dir))
	if len(after) != 2 || after[filepath.Join("suite", wire.CallsFile)] != before[filepath.Join("suite", wire.CallsFile)] || after[filepath.Join("suite", wire.IndexFile)] != before[filepath.Join("suite", wire.IndexFile)] {
		t.Fatalf("the record changed: %v", slices.Collect(maps.Keys(after)))
	}
	if status := summaryWire(t, dir); status.State != wire.StateTaken || status.Calls != 2 {
		t.Fatalf("summary.wire %+v", status)
	}
	absent(t, stagedRecord(dir))
	if got := len(w.fake.Wire.Taken()); got != asked || asked != 1 {
		t.Fatalf("takes %d after %d", got, asked)
	}
}

func TestADiscardedBundleLeavesTheRecordReadableInTheRunDirectory(t *testing.T) {
	w := wireWorld(t, nil)
	w.hub.o.Reporter, w.hub.ops.Reporter = plantInFrozenCopy, plantInFrozenCopy
	m, dir := w.reportedWire()
	if m.Redaction == nil || m.Redaction.Status != report.RedactionFailed {
		t.Fatalf("redaction %+v", m.Redaction)
	}
	absent(t, filepath.Join(dir, report.DirReport))
	present(t, filepath.Join(stagedRecord(dir), wire.SuiteTake, wire.IndexFile))
	store := wire.OpenStore(dir)
	if store.Root() != stagedRecord(dir) {
		t.Fatalf("root %s", store.Root())
	}
	index, err := store.Index(wire.SuiteTake)
	if err != nil || index.Status.State != wire.StateTaken || len(index.Heads) != 2 {
		t.Fatalf("index %+v: %v", index, err)
	}
	if r := w.sched.Destroy(m.ID, tester); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	present(t, filepath.Join(stagedRecord(dir), wire.SuiteTake, wire.CallsFile))
	absent(t, filepath.Join(dir, "work"))
}

func TestDestroyKeepsTheRecordInTheBundleAndMovesAStrandedOneThere(t *testing.T) {
	w := wireWorld(t, nil)
	m, dir := w.reportedWire()
	present(t, filepath.Join(bundleRecord(dir), wire.SuiteTake, wire.CallsFile))
	if err := wire.OpenStore(dir).Detach(); err != nil {
		t.Fatal(err)
	}
	present(t, filepath.Join(stagedRecord(dir), wire.SuiteTake, wire.CallsFile))
	absent(t, bundleRecord(dir))
	if r := w.sched.Destroy(m.ID, tester); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	if got := w.meta(m.ID); got.Env != lifecycle.EnvDestroyed {
		t.Fatalf("state %+v", got.State)
	}
	present(t, filepath.Join(bundleRecord(dir), wire.SuiteTake, wire.CallsFile))
	absent(t, stagedRecord(dir))
	index, err := wire.OpenStore(dir).Index(wire.SuiteTake)
	if err != nil || len(index.Heads) != 2 {
		t.Fatalf("after destroy: %+v %v", index, err)
	}
}

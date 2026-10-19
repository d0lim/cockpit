package scheduler

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
)

func TestSecondSubmitWhileActiveQueuesWithAPositionAndStartsWhenTheFirstFinishes(t *testing.T) {
	s, w := setup(t, nil)
	first := s.Submit(ctx, sub("c1"))
	s.Tick()
	if first.Fail != nil || first.Phase != lifecycle.Queued || first.Position != 1 || first.Merged {
		t.Fatalf("%+v", first)
	}
	if m := read(t, w, first.ID); m.Phase != lifecycle.Preparing || *m.Slot != 0 {
		t.Fatalf("%+v", m)
	}
	second := s.Submit(ctx, sub("c2"))
	s.Tick()
	if second.Position != 1 || s.WaitReasonOf(second.ID) != lifecycle.WaitActiveRun || s.Get(second.ID).Position != 1 {
		t.Fatalf("%+v %q", second, s.WaitReasonOf(second.ID))
	}
	until(t, func() bool { return len(w.startedRuns()) > 0 })
	if !slices.Equal(w.startedRuns(), []string{first.ID}) {
		t.Fatalf("%v", w.startedRuns())
	}
	w.finish(first.ID, 30)
	until(t, func() bool { return len(w.startedRuns()) == 2 })
	if !slices.Equal(w.startedRuns(), []string{first.ID, second.ID}) {
		t.Fatalf("%v", w.startedRuns())
	}
	until(t, func() bool { return read(t, w, second.ID).Phase == lifecycle.Preparing })
	if m := read(t, w, second.ID); *m.Slot != 1 || s.WaitReasonOf(second.ID) != lifecycle.WaitNone {
		t.Fatalf("%+v", m)
	}
}

func TestSubmitOfTheSameCommitAndSuiteMergesWhileQueuedAndNewQueuesAnotherRun(t *testing.T) {
	s, _ := setup(t, nil)
	s.Submit(ctx, sub("c0"))
	s.Tick()
	first := s.Submit(ctx, sub("c1"))
	again := s.Submit(ctx, sub("c1", func(in *SubmitInput) { in.Submitter = "ui" }))
	fresh := s.Submit(ctx, sub("c1", func(in *SubmitInput) { in.New = true }))
	if again.ID != first.ID || !again.Merged || again.Position != 1 {
		t.Fatalf("%+v", again)
	}
	if fresh.Merged || fresh.Position != 2 || fresh.ID == first.ID {
		t.Fatalf("%+v", fresh)
	}
}

func TestEleventhQueuedSubmitIsRefusedWithQueueFull(t *testing.T) {
	s, _ := setup(t, nil)
	s.Submit(ctx, sub("active"))
	s.Tick()
	for n := range 10 {
		if r := s.Submit(ctx, sub("c"+string(rune('a'+n)))); r.Fail != nil {
			t.Fatalf("submit %d: %+v", n, r.Fail)
		}
	}
	r := s.Submit(ctx, sub("c-overflow"))
	if r.Fail == nil || r.Fail.Status != 409 || r.Fail.Code != ticker.CodeQueueFull {
		t.Fatalf("%+v", r)
	}
}

func TestSubmitRejectionComesBackWithAStableCodeAndTheFileList(t *testing.T) {
	s, w := setup(t, nil, func(d *Deps) {
		d.Accept = func(_ context.Context, _ SubmitInput) (Accepted, error) {
			return Accepted{}, &Rejection{Kind: KindDirty, Message: "commit first", Files: []string{"app/x.go"}}
		}
	})
	r := s.Submit(ctx, sub("c1"))
	if r.Fail == nil || r.Fail.Status != 400 || r.Fail.Code != ticker.CodeRejectedDirty || !slices.Equal(r.Fail.Files, []string{"app/x.go"}) {
		t.Fatalf("%+v", r.Fail)
	}
	list, _ := w.store.ListRuns()
	if len(list.Runs) != 0 {
		t.Fatalf("runs %d", len(list.Runs))
	}
}

func TestSubmitWritesTheCockpitVersionAndHostAPIIntoTheRunMeta(t *testing.T) {
	s, w := setup(t, nil)
	r := s.Submit(ctx, sub("c1"))
	m := read(t, w, r.ID)
	if m.CockpitVersion != "v0.3.0" || m.HostAPI != 1 || m.SHA != shaOf("c1") || m.Suite != "smoke" || m.Submitter != "cli" {
		t.Fatalf("%+v", m)
	}
}

func TestSubmitWhileDockerIsSilentWaitsAsDockerUnavailableAndStartsAfterRecovery(t *testing.T) {
	s, w := setup(t, nil, func(d *Deps) { d.TickInterval = 20 * time.Millisecond })
	w.dockerUp = false
	r := s.Submit(ctx, sub("c1"))
	s.Tick()
	list, _ := s.List()
	if s.WaitReasonOf(r.ID) != lifecycle.WaitDockerUnavailable || !list.DockerDown || len(w.startedRuns()) != 0 {
		t.Fatalf("%q %v %v", s.WaitReasonOf(r.ID), list.DockerDown, w.startedRuns())
	}
	w.mu.Lock()
	w.dockerUp = true
	w.mu.Unlock()
	until(t, func() bool { return len(w.startedRuns()) == 1 })
	if !slices.Equal(w.startedRuns(), []string{r.ID}) {
		t.Fatalf("%v", w.startedRuns())
	}
}

func TestReconciliationWaitsForTheDaemonAndRunsOnceAfterItRecovers(t *testing.T) {
	s, w := setup(t, []store.Meta{preserved(1, 10)}, func(d *Deps) { d.TickInterval = 20 * time.Millisecond })
	reconciles := func() int { return len(suffixed(w.snapshot(), "docker labelled "+rid(1))) }
	w.dockerUp = false
	s.Tick()
	if reconciles() != 0 {
		t.Fatalf("reconciled while down: %d", reconciles())
	}
	w.mu.Lock()
	w.dockerUp = true
	w.mu.Unlock()
	until(t, func() bool { return reconciles() == 1 })
	s.Tick()
	s.Tick()
	list, _ := s.List()
	if reconciles() != 1 || list.DockerDown {
		t.Fatalf("reconciles %d down %v", reconciles(), list.DockerDown)
	}
}

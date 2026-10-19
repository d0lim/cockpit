package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/d0lim/cockpit/internal/hub/lease"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
)

func (w *world) apply(id string, e lifecycle.Event, extra func(*store.Meta)) ticker.Result {
	m, err := w.store.ApplyEvent(id, e, lifecycle.Detail{}, func(x *store.Meta) {
		x.Pgid = nil
		if extra != nil {
			extra(x)
		}
	})
	if err != nil {
		return ticker.Conflict(err.Error(), &m)
	}
	return ticker.Succeeded(m)
}

func (w *world) release(id string) {
	dir, _ := w.store.RunDir(id)
	for _, name := range []string{"src", "work", "env", "build"} {
		os.RemoveAll(filepath.Join(dir, name))
	}
	w.store.UpdateState(func(s store.HubState) store.HubState { s.Leases = lease.Release(s.Leases, id); return s })
}

func (w *world) Stop(_ context.Context, id string) ticker.Result {
	w.note(id + " stop")
	return w.apply(id, lifecycle.Stopped, nil)
}

func (w *world) Destroy(_ context.Context, id string) ticker.Result {
	w.note(id + " down")
	w.release(id)
	return w.apply(id, lifecycle.Destroyed, nil)
}

func (w *world) Resume(_ context.Context, id string) ticker.Result {
	m, _ := w.store.MustRead(id)
	if m.Recreate {
		w.note(id + " up --force-recreate")
	} else {
		w.note(id + " start")
	}
	w.note("awaitBoot")
	return w.apply(id, lifecycle.Resumed, func(x *store.Meta) { x.LastUpAt = store.Timestamp(w.now()); x.Recreate = false })
}

func (w *world) KillGroup(pgid int) error { w.note(fmt.Sprintf("kill %d", pgid)); return nil }

func (w *world) RemoveOneOffs(_ context.Context, id string) error {
	w.note(id + " removeOneOffs")
	return nil
}

func (w *world) CloseWithoutEnv(_ context.Context, id string, down bool) error {
	if down {
		w.note(id + " down")
	}
	w.release(id)
	return nil
}

func (w *world) StopQuietly(_ context.Context, id string) { w.note(id + " stop") }

func (w *world) HasRunnerResult(string) bool { return false }

func (w *world) Report(context.Context, store.Meta) (reconcile.ReportOutcome, error) {
	return reconcile.ReportOutcome{Verdict: lifecycle.VerdictPassed}, nil
}

func (w *world) Redo(ctx context.Context, id string) error {
	m, _ := w.store.MustRead(id)
	switch m.Phase {
	case lifecycle.Stopping:
		w.Stop(ctx, id)
	case lifecycle.Resuming:
		w.Resume(ctx, id)
	case lifecycle.Destroying:
		w.Destroy(ctx, id)
	}
	return nil
}

func (w *world) Mask(_, text string) string { return text }

func (w *world) startedCh(id string) chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	ch, ok := w.starts[id]
	if !ok {
		ch = make(chan struct{})
		w.starts[id] = ch
	}
	return ch
}

func (w *world) pipeline(ctx context.Context, id string) error {
	started := w.startedCh(id)
	w.mu.Lock()
	w.started = append(w.started, id)
	done := make(chan struct{})
	w.finishes[id] = done
	w.mu.Unlock()
	close(started)
	w.note(id + " pipeline")
	select {
	case <-done:
	case <-ctx.Done():
		w.store.PatchMeta(id, func(m *store.Meta) {
			if m.Phase != lifecycle.Destroying {
				m.State = lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictAborted}
				m.FinishedAt, m.LastUpAt = at(50), at(50)
			}
		})
	}
	return nil
}

func (w *world) finish(id string, minute int) {
	select {
	case <-w.startedCh(id):
	case <-time.After(5 * time.Second):
		w.t.Fatalf("the pipeline of %s never started", id)
	}
	w.store.PatchMeta(id, func(m *store.Meta) {
		m.State = lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictPassed}
		m.FinishedAt, m.LastUpAt = at(minute), at(minute)
	})
	w.mu.Lock()
	close(w.finishes[id])
	w.mu.Unlock()
}

func (w *world) startedRuns() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.started)
}

func (w *world) Denied(m store.Meta, _ bool, _ string) *ticker.Failure {
	if m.Env != lifecycle.EnvUp {
		return &ticker.Failure{Status: 409, Code: ticker.CodeConflictState, Reason: "the environment is stopped: resume first"}
	}
	return nil
}

func (w *world) Run(_ context.Context, m store.Meta, flow string) ExecResponse {
	w.note(m.ID + " runRunner " + flow)
	return ExecResponse{Result: flow}
}

package hubtest

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/pool"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
)

const ToySHA = "0123456789abcdef0123456789abcdef01234567"

var walk = []lifecycle.Event{lifecycle.Prepared, lifecycle.Built, lifecycle.Sealed, lifecycle.Booted, lifecycle.Tested, lifecycle.Reported}

type World struct {
	mu      sync.Mutex
	Store   *store.Store
	Verdict lifecycle.Verdict
	Hold    chan struct{}
	Before  func(id string)
	Exec    any
	Calls   []string
}

func New(st *store.Store) *World {
	return &World{Store: st, Verdict: lifecycle.VerdictPassed}
}

func (w *World) note(s string) {
	w.mu.Lock()
	w.Calls = append(w.Calls, s)
	w.mu.Unlock()
}

func ShaOf(commit string) string {
	if len(commit) == 40 {
		return commit
	}
	sum := sha1.Sum([]byte(commit))
	return hex.EncodeToString(sum[:])
}

func (w *World) Deps() scheduler.Deps {
	return scheduler.Deps{
		Deps: ticker.Deps{
			Store: w.Store, Docker: w, Env: w, Pipeline: w.Pipeline, Limits: pool.DefaultLimits(),
			TickInterval: 20 * time.Millisecond, Now: time.Now, Served: []int{1}, CockpitVersion: "0.1.0",
			BootTime: func() time.Time { return time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC) },
			Lease:    w.Lease,
		},
		Accept: func(_ context.Context, in scheduler.SubmitInput) (scheduler.Accepted, error) {
			if in.Commit == "bad" {
				return scheduler.Accepted{}, &scheduler.Rejection{Kind: scheduler.KindCommit, Message: "commit bad does not resolve"}
			}
			return scheduler.Accepted{SHA: ShaOf(in.Commit), HostAPI: 1}, nil
		},
		Exec: w,
	}
}

func (w *World) Lease(leases store.Leases, _ string) (int, error) {
	used := map[int]bool{}
	for _, s := range leases {
		used[s] = true
	}
	for slot := range 8 {
		if !used[slot] {
			return slot, nil
		}
	}
	return 0, errors.New("no port block is free")
}

func (w *World) Pipeline(ctx context.Context, id string) error {
	w.note("pipeline " + id)
	if w.Before != nil {
		w.Before(id)
	}
	if w.Hold != nil {
		select {
		case <-w.Hold:
		case <-ctx.Done():
			_, err := w.Store.ApplyEvent(id, lifecycle.Abort, lifecycle.Detail{}, nil)
			return err
		}
	}
	for _, e := range walk {
		d := lifecycle.Detail{}
		if e == lifecycle.Reported {
			d.Verdict = w.Verdict
		}
		if _, err := w.Store.ApplyEvent(id, e, d, func(m *store.Meta) {
			if e == lifecycle.Reported {
				m.FinishedAt = store.Timestamp(time.Now())
				m.LastUpAt = m.FinishedAt
			}
		}); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) apply(id string, e lifecycle.Event) ticker.Result {
	m, err := w.Store.ApplyEvent(id, e, lifecycle.Detail{}, nil)
	if err != nil {
		return ticker.Conflict(err.Error(), &m)
	}
	return ticker.Succeeded(m)
}

func (w *World) envs() []store.Meta {
	list, _ := w.Store.ListRuns()
	var out []store.Meta
	for _, m := range list.Runs {
		if m.Mark == lifecycle.MarkNone && (m.Env == lifecycle.EnvUp || m.Env == lifecycle.EnvStopped) {
			out = append(out, m)
		}
	}
	return out
}

func (w *World) ComposeLs(context.Context) ([]reconcile.Project, error) {
	out := []reconcile.Project{}
	for _, m := range w.envs() {
		status := "exited(3)"
		if m.Env == lifecycle.EnvUp {
			status = "running(3)"
		}
		out = append(out, reconcile.Project{Name: m.ID, Status: status})
	}
	return out, nil
}

func (w *World) Info(context.Context) bool { return true }

func (w *World) Labelled(_ context.Context, project string) (reconcile.Labelled, error) {
	for _, m := range w.envs() {
		if m.ID == project {
			return reconcile.Labelled{Containers: 3, Networks: 1}, nil
		}
	}
	return reconcile.Labelled{}, nil
}

func (w *World) Stop(_ context.Context, id string) ticker.Result {
	w.note("stop " + id)
	return w.apply(id, lifecycle.Stopped)
}

func (w *World) Destroy(_ context.Context, id string) ticker.Result {
	w.note("destroy " + id)
	return w.apply(id, lifecycle.Destroyed)
}

func (w *World) Resume(_ context.Context, id string) ticker.Result {
	w.note("resume " + id)
	return w.apply(id, lifecycle.Resumed)
}

func (w *World) KillGroup(int) error                                 { return nil }
func (w *World) RemoveOneOffs(context.Context, string) error         { return nil }
func (w *World) CloseWithoutEnv(context.Context, string, bool) error { return nil }
func (w *World) StopQuietly(context.Context, string)                 {}
func (w *World) HasRunnerResult(string) bool                         { return false }
func (w *World) Redo(context.Context, string) error                  { return nil }
func (w *World) Mask(_, text string) string                          { return text }
func (w *World) Report(context.Context, store.Meta) (reconcile.ReportOutcome, error) {
	return reconcile.ReportOutcome{Verdict: w.Verdict}, nil
}

func (w *World) Denied(m store.Meta, busy bool, flow string) *ticker.Failure {
	if !pool.Stoppable(m) {
		return &ticker.Failure{Status: 409, Code: ticker.CodeConflictState, Reason: "exec needs a finished run with its environment up", Run: &m}
	}
	if flow == "missing" {
		return &ticker.Failure{Status: 404, Code: ticker.CodeNotFound, Reason: "the manifest has no flow missing"}
	}
	return nil
}

func (w *World) Run(_ context.Context, _ store.Meta, flow string) scheduler.ExecResponse {
	w.note("exec " + flow)
	return scheduler.ExecResponse{Result: w.Exec}
}

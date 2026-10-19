package reconcile

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
)

const planted = "PLANTED-cred-7f3a9c"

func rid(n int) string { return fmt.Sprintf("run-20261012-1530%02d-997a37e", n) }

func at(minute int) string { return fmt.Sprintf("2026-10-12T06:%02d:00.000Z", minute) }

func meta(id string, edit ...func(*store.Meta)) store.Meta {
	m := store.Meta{State: lifecycle.State{Phase: lifecycle.Queued, Env: lifecycle.EnvAbsent}, ID: id, SHA: "997a37e0c1d2b3a4f5e6d7c8b9a0f1e2d3c4b5a6", Suite: "smoke", HostAPI: 1, CockpitVersion: "v0.3.0", Submitter: "cli", CreatedAt: at(0)}
	for _, e := range edit {
		e(&m)
	}
	return m
}

func withSlot(n int) func(*store.Meta) { return func(m *store.Meta) { m.Slot = &n } }

func preserved(n, minute int, edit ...func(*store.Meta)) store.Meta {
	base := func(m *store.Meta) {
		m.CreatedAt, m.StartedAt, m.FinishedAt, m.LastUpAt = at(minute), at(minute), at(minute), at(minute)
		m.State = lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictPassed}
	}
	return meta(rid(n), append([]func(*store.Meta){base, withSlot(n)}, edit...)...)
}

type fakeDocker struct {
	mu       sync.Mutex
	store    *store.Store
	ls       []Project
	labelled map[string]Labelled
	calls    []string
}

func holdsEnv(m store.Meta) bool {
	return m.Mark == lifecycle.MarkNone && (m.Env == lifecycle.EnvUp || m.Env == lifecycle.EnvStopped)
}

func (f *fakeDocker) withEnv() []store.Meta {
	list, _ := f.store.ListRuns()
	var out []store.Meta
	for _, m := range list.Runs {
		if holdsEnv(m) {
			out = append(out, m)
		}
	}
	return out
}

func (f *fakeDocker) ComposeLs(context.Context) ([]Project, error) {
	f.mu.Lock()
	ls := f.ls
	f.calls = append(f.calls, "composeLs")
	f.mu.Unlock()
	if ls != nil {
		return ls, nil
	}
	out := []Project{}
	for _, m := range f.withEnv() {
		status := "exited(12)"
		if m.Env == lifecycle.EnvUp {
			status = "running(12)"
		}
		out = append(out, Project{Name: m.ID, Status: status})
	}
	return out, nil
}

func (f *fakeDocker) Info(context.Context) bool { return true }

func (f *fakeDocker) Labelled(_ context.Context, project string) (Labelled, error) {
	f.mu.Lock()
	f.calls = append(f.calls, "labelled "+project)
	l, ok := f.labelled[project]
	f.mu.Unlock()
	if ok {
		return l, nil
	}
	if slices.ContainsFunc(f.withEnv(), func(m store.Meta) bool { return m.ID == project }) {
		return Labelled{Containers: 12, Networks: 2, Volumes: 3}, nil
	}
	return Labelled{}, nil
}

type fakeEnv struct {
	mu        sync.Mutex
	calls     []string
	killed    []int
	results   map[string]bool
	reportErr error
	killErr   error
	removeErr error
	reports   []store.Meta
}

func (f *fakeEnv) note(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fakeEnv) KillGroup(pgid int) error {
	f.mu.Lock()
	f.killed = append(f.killed, pgid)
	f.mu.Unlock()
	f.note(fmt.Sprintf("kill %d", pgid))
	return f.killErr
}

func (f *fakeEnv) RemoveOneOffs(_ context.Context, id string) error {
	f.note(id + " removeOneOffs")
	return f.removeErr
}

func (f *fakeEnv) CloseWithoutEnv(_ context.Context, id string, down bool) error {
	f.note(fmt.Sprintf("%s closeWithoutEnv down=%v", id, down))
	return nil
}

func (f *fakeEnv) StopQuietly(_ context.Context, id string) { f.note(id + " stopQuietly") }

func (f *fakeEnv) HasRunnerResult(id string) bool { return f.results[id] }

func (f *fakeEnv) Report(_ context.Context, m store.Meta) (ReportOutcome, error) {
	f.mu.Lock()
	f.reports = append(f.reports, m)
	err := f.reportErr
	f.mu.Unlock()
	f.note(m.ID + " report")
	if err != nil {
		return ReportOutcome{}, err
	}
	return ReportOutcome{Verdict: lifecycle.VerdictPassed}, nil
}

func (f *fakeEnv) Redo(_ context.Context, id string) error {
	f.note(id + " redo")
	return nil
}

func (f *fakeEnv) Mask(_, text string) string { return strings.ReplaceAll(text, planted, "****") }

type world struct {
	store  *store.Store
	docker *fakeDocker
	env    *fakeEnv
	deps   Deps
	hooks  Hooks
}

func setup(t *testing.T, metas ...store.Meta) *world {
	t.Helper()
	s := store.Open(filepath.Join(t.TempDir(), "home"))
	for _, m := range metas {
		if err := s.WriteMeta(m); err != nil {
			t.Fatal(err)
		}
	}
	listed, _ := s.ListRuns()
	if err := s.WriteState(store.RebuildState(listed.Runs)); err != nil {
		t.Fatal(err)
	}
	w := &world{store: s, docker: &fakeDocker{store: s, labelled: map[string]Labelled{}}, env: &fakeEnv{results: map[string]bool{}}}
	clock := time.Date(2026, 10, 12, 7, 0, 0, 0, time.UTC)
	w.deps = Deps{
		Store: s, Docker: w.docker, Env: w.env,
		Now:            func() time.Time { clock = clock.Add(time.Second); return clock },
		BootTime:       func() time.Time { return time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC) },
		Served:         []int{1},
		CockpitVersion: "v0.3.0",
	}
	w.hooks = Hooks{Skip: func(string) bool { return false }, RunOp: func(_ string, fn func() error) error { return fn() }, Managed: NewManaged()}
	return w
}

func (w *world) run(t *testing.T) Scan {
	t.Helper()
	scan, err := Run(context.Background(), w.deps, w.hooks)
	if err != nil {
		t.Fatal(err)
	}
	return scan
}

func (w *world) read(t *testing.T, id string) store.Meta {
	t.Helper()
	m, err := w.store.MustRead(id)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func own(calls []string, id string) []string {
	var out []string
	for _, c := range calls {
		if strings.HasPrefix(c, id+" ") {
			out = append(out, c)
		}
	}
	return out
}

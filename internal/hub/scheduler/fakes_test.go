package scheduler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/hub/lease"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/pool"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
)

func rid(n int) string { return fmt.Sprintf("run-20261012-1530%02d-997a37e", n) }

func at(minute int) string { return fmt.Sprintf("2026-10-12T06:%02d:00.000Z", minute) }

func shaOf(commit string) string {
	sum := sha1.Sum([]byte(commit))
	return hex.EncodeToString(sum[:])
}

func meta(id string, edit ...func(*store.Meta)) store.Meta {
	m := store.Meta{State: lifecycle.State{Phase: lifecycle.Queued, Env: lifecycle.EnvAbsent}, ID: id, SHA: shaOf("c0"), Suite: "smoke", HostAPI: 1, CockpitVersion: "v0.3.0", Submitter: "cli", CreatedAt: at(0)}
	for _, e := range edit {
		e(&m)
	}
	return m
}

func preserved(n, minute int, edit ...func(*store.Meta)) store.Meta {
	slot := n
	base := func(m *store.Meta) {
		m.CreatedAt, m.StartedAt, m.FinishedAt, m.LastUpAt = at(minute), at(minute), at(minute), at(minute)
		m.State = lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictPassed}
		m.Slot = &slot
	}
	return meta(rid(n), append([]func(*store.Meta){base}, edit...)...)
}

func kept(m *store.Meta) { m.Keep = true }

type world struct {
	mu       sync.Mutex
	t        *testing.T
	store    *store.Store
	calls    []string
	dockerUp bool
	foreign  []reconcile.Project
	onLs     func()
	labelled map[string]int
	started  []string
	finishes map[string]chan struct{}
	blocked  []int
	clock    time.Time
	execs    []string
}

func (w *world) note(s string) {
	w.mu.Lock()
	w.calls = append(w.calls, s)
	w.mu.Unlock()
}

func (w *world) snapshot() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.calls)
}

func (w *world) now() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.clock = w.clock.Add(time.Second)
	return w.clock
}

func (w *world) withEnv() []store.Meta {
	list, _ := w.store.ListRuns()
	var out []store.Meta
	for _, m := range list.Runs {
		if m.Mark == lifecycle.MarkNone && (m.Env == lifecycle.EnvUp || m.Env == lifecycle.EnvStopped) {
			out = append(out, m)
		}
	}
	return out
}

func (w *world) ComposeLs(context.Context) ([]reconcile.Project, error) {
	if w.onLs != nil {
		w.onLs()
	}
	out := []reconcile.Project{}
	for _, m := range w.withEnv() {
		status := "exited(12)"
		if m.Env == lifecycle.EnvUp {
			status = "running(12)"
		}
		out = append(out, reconcile.Project{Name: m.ID, Status: status})
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return append(out, w.foreign...), nil
}

func (w *world) Info(context.Context) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.dockerUp
}

func (w *world) Labelled(_ context.Context, project string) (reconcile.Labelled, error) {
	w.note("docker labelled " + project)
	if slices.ContainsFunc(w.withEnv(), func(m store.Meta) bool { return m.ID == project }) {
		return reconcile.Labelled{Containers: 12, Networks: 2, Volumes: 3}, nil
	}
	return reconcile.Labelled{}, nil
}

func setup(t *testing.T, metas []store.Meta, edit ...func(*Deps)) (*Scheduler, *world) {
	t.Helper()
	s := store.Open(filepath.Join(t.TempDir(), "home"))
	for _, m := range metas {
		if err := s.WriteMeta(m); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := s.ListRuns()
	s.WriteState(store.RebuildState(list.Runs))
	w := &world{t: t, store: s, dockerUp: true, finishes: map[string]chan struct{}{}, clock: time.Date(2026, 10, 12, 7, 0, 0, 0, time.UTC)}
	layout := lease.DefaultLayout(map[string]int{"APP": 0, "API": 1})
	d := Deps{
		Deps: ticker.Deps{
			Store: s, Docker: w, Env: w, Pipeline: w.pipeline, Limits: pool.DefaultLimits(), StopOnDone: true,
			TickInterval: 10 * time.Second, Now: w.now, Served: []int{1}, CockpitVersion: "v0.3.0",
			BootTime: func() time.Time { return time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC) },
			Lease: func(leases store.Leases, id string) (int, error) {
				w.note("leaseBlock")
				_, slot, _, err := lease.Lease(leases, id, func(p int) bool {
					w.mu.Lock()
					defer w.mu.Unlock()
					return !slices.Contains(w.blocked, p)
				}, layout)
				return slot, err
			},
		},
		Accept: func(_ context.Context, in SubmitInput) (Accepted, error) {
			return Accepted{SHA: shaOf(in.Commit), HostAPI: 1}, nil
		},
		Exec: w,
	}
	for _, e := range edit {
		e(&d)
	}
	sch := New(d)
	t.Cleanup(func() { sch.Dispose(); sch.Idle() })
	return sch, w
}

func sub(commit string, edit ...func(*SubmitInput)) SubmitInput {
	in := SubmitInput{Worktree: "/fake/hub", Commit: commit, Suite: "smoke", Submitter: "cli"}
	for _, e := range edit {
		e(&in)
	}
	return in
}

func until(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 3s")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func read(t *testing.T, w *world, id string) store.Meta {
	t.Helper()
	m, err := w.store.MustRead(id)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func suffixed(calls []string, suffix string) []string {
	var out []string
	for _, c := range calls {
		if strings.HasSuffix(c, suffix) {
			out = append(out, c)
		}
	}
	return out
}

func inOrder(calls []string, expected ...string) bool {
	i := 0
	for _, c := range calls {
		if i < len(expected) && c == expected[i] {
			i++
		}
	}
	return i == len(expected)
}

var ctx = context.Background()

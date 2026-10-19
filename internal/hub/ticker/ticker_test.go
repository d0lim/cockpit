package ticker

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/hub/lease"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/pool"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/store"
)

const planted = "PLANTED-cred-7f3a9c"

type gateDocker struct {
	infos atomic.Int32
	gate  chan struct{}
}

func (g *gateDocker) ComposeLs(context.Context) ([]reconcile.Project, error) {
	return []reconcile.Project{}, nil
}

func (g *gateDocker) Info(context.Context) bool {
	g.infos.Add(1)
	if g.gate != nil {
		<-g.gate
	}
	return true
}

func (g *gateDocker) Labelled(context.Context, string) (reconcile.Labelled, error) {
	return reconcile.Labelled{}, nil
}

type quietEnv struct {
	mu     sync.Mutex
	closed []string
}

func (e *quietEnv) KillGroup(int) error                         { return nil }
func (e *quietEnv) RemoveOneOffs(context.Context, string) error { return nil }
func (e *quietEnv) StopQuietly(context.Context, string)         {}
func (e *quietEnv) HasRunnerResult(string) bool                 { return false }
func (e *quietEnv) Redo(context.Context, string) error          { return nil }
func (e *quietEnv) Stop(context.Context, string) Result         { return Result{} }
func (e *quietEnv) Destroy(context.Context, string) Result      { return Result{} }
func (e *quietEnv) Resume(context.Context, string) Result       { return Result{} }
func (e *quietEnv) Mask(_, text string) string                  { return strings.ReplaceAll(text, planted, "****") }
func (e *quietEnv) Report(context.Context, store.Meta) (reconcile.ReportOutcome, error) {
	return reconcile.ReportOutcome{}, nil
}

func (e *quietEnv) CloseWithoutEnv(_ context.Context, id string, _ bool) error {
	e.mu.Lock()
	e.closed = append(e.closed, id)
	e.mu.Unlock()
	return nil
}

const queuedID = "run-20261012-153001-997a37e"

func core(t *testing.T, docker reconcile.Docker, env *quietEnv, edit ...func(*Deps)) (*Core, *store.Store) {
	t.Helper()
	s := store.Open(filepath.Join(t.TempDir(), "home"))
	m := store.Meta{State: lifecycle.State{Phase: lifecycle.Queued, Env: lifecycle.EnvAbsent}, ID: queuedID, HostAPI: 1, Submitter: "cli", CreatedAt: "2026-10-12T06:00:00.000Z"}
	if err := s.WriteMeta(m); err != nil {
		t.Fatal(err)
	}
	s.WriteState(store.HubState{SchemaVersion: store.StateSchemaVersion, Queue: []string{queuedID}, Leases: store.Leases{}})
	d := Deps{
		Store: s, Docker: docker, Env: env, Limits: pool.DefaultLimits(), TickInterval: time.Hour, Served: []int{1},
		Now:      func() time.Time { return time.Date(2026, 10, 12, 7, 0, 0, 0, time.UTC) },
		BootTime: func() time.Time { return time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC) },
		Lease:    func(store.Leases, string) (int, error) { return 0, nil },
		Pipeline: func(context.Context, string) error { return nil },
	}
	for _, e := range edit {
		e(&d)
	}
	c := New(d)
	t.Cleanup(func() { c.Dispose(); c.Idle() })
	return c, s
}

func TestPipelineErrorClosesTheRunAsErrorAndMasksThePlantedSecretInTheNotice(t *testing.T) {
	env := &quietEnv{}
	c, s := core(t, &gateDocker{}, env, func(d *Deps) {
		d.Pipeline = func(context.Context, string) error { return errors.New("compose crashed with token " + planted) }
	})
	c.Tick()
	c.Idle()
	m, _ := s.MustRead(queuedID)
	if m.Phase != lifecycle.Done || m.Env != lifecycle.EnvAbsent || m.Verdict != lifecycle.VerdictError {
		t.Fatalf("%+v", m.State)
	}
	if strings.Contains(m.Notice, planted) || !strings.Contains(m.Notice, "the pipeline stopped: compose crashed with token ****") {
		t.Fatalf("notice %q", m.Notice)
	}
	if len(env.closed) != 1 || env.closed[0] != queuedID {
		t.Fatalf("closed %v", env.closed)
	}
}

func TestTicksRequestedWhileATickRunsFoldIntoOneMoreRound(t *testing.T) {
	docker := &gateDocker{gate: make(chan struct{})}
	c, _ := core(t, docker, &quietEnv{}, func(d *Deps) {
		d.Pipeline = func(ctx context.Context, _ string) error { <-ctx.Done(); return nil }
	})
	go c.Tick()
	for docker.infos.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(c.Tick)
	}
	time.Sleep(20 * time.Millisecond)
	close(docker.gate)
	wg.Wait()
	if got := docker.infos.Load(); got != 2 {
		t.Fatalf("docker info called %d times, want 2", got)
	}
}

func TestQueueHeadWaitsWithNoPortBlockWhenEveryBlockIsTaken(t *testing.T) {
	c, s := core(t, &gateDocker{}, &quietEnv{}, func(d *Deps) {
		d.Lease = func(store.Leases, string) (int, error) { return 0, lease.ErrNoPortBlock }
	})
	c.Tick()
	m, _ := s.MustRead(queuedID)
	if m.Phase != lifecycle.Queued || m.WaitReason != lifecycle.WaitNoPortBlock {
		t.Fatalf("%+v", m)
	}
}

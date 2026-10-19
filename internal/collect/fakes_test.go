package collect_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/collect/collecttest"
	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports"
)

const (
	supPlanted = "PLANTED-collect-3d9f"
	supRun     = "run-1"
	supProject = "cockpit-run-1"
)

type runRecord struct {
	collector  string
	deadline   time.Duration
	start, end time.Time
}

type fakeRunner struct {
	mu        sync.Mutex
	runs      []runRecord
	active    int
	maxActive int
	fn        func(ctx context.Context, spec worker.Spec) (worker.Outcome, error)
}

func (f *fakeRunner) run(ctx context.Context, spec worker.Spec) (worker.Outcome, error) {
	f.mu.Lock()
	f.active++
	f.maxActive = max(f.maxActive, f.active)
	rec := runRecord{collector: spec.Collector, deadline: spec.Limits.Deadline, start: time.Now()}
	f.mu.Unlock()
	out, err := f.fn(ctx, spec)
	f.mu.Lock()
	f.active--
	rec.end = time.Now()
	f.runs = append(f.runs, rec)
	f.mu.Unlock()
	return out, err
}

func (f *fakeRunner) records(collector string) []runRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []runRecord
	for _, r := range f.runs {
		if collector == "" || r.collector == collector {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeRunner) peak() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxActive
}

func waitOrCancel(ctx context.Context, d time.Duration) worker.Outcome {
	select {
	case <-time.After(d):
		return worker.Outcome{Status: worker.StatusOK}
	case <-ctx.Done():
		return worker.Outcome{Status: worker.StatusKilled, Kill: worker.KillCanceled}
	}
}

type memEmitter struct {
	mu   sync.Mutex
	sigs []contract.Signal
}

func (m *memEmitter) Append(sig contract.Signal) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sigs = append(m.sigs, sig)
	return int64(len(m.sigs)), nil
}

func (m *memEmitter) byCollector(id string) []contract.Signal {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []contract.Signal
	for _, s := range m.sigs {
		if s.Collector == id {
			out = append(out, s)
		}
	}
	return out
}

func noPorts(string) map[string]ports.Handler { return map[string]ports.Handler{} }

func newSupervisor(f *fakeRunner, opts collect.Options) *collect.Supervisor {
	if f != nil {
		opts.Runner = f.run
	}
	if opts.Command == nil {
		opts.Command = collecttest.Command()
	}
	return collect.New(opts)
}

func register(t *testing.T, s *collect.Supervisor, cfg collect.RunConfig) {
	t.Helper()
	if cfg.RunID == "" {
		cfg.RunID = supRun
	}
	if cfg.Ports == nil {
		cfg.Ports = noPorts
	}
	if cfg.Mask == nil {
		cfg.Mask = collecttest.MaskOf(supPlanted)
	}
	if err := s.Register(cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Drain(context.Background(), cfg.RunID, lifecycle.CauseDestroying) })
}

func arm(t *testing.T, s *collect.Supervisor, cause lifecycle.HookCause) {
	t.Helper()
	if err := s.Arm(context.Background(), supRun, cause); err != nil {
		t.Fatal(err)
	}
}

func eventually(t *testing.T, within time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

type fakeDocker struct {
	mu     sync.Mutex
	calls  [][]string
	ps     []string
	handle func(ctx context.Context, cmd compose.Command) (compose.Result, error)
}

func (f *fakeDocker) Run(ctx context.Context, cmd compose.Command) (compose.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), cmd.Args...))
	f.mu.Unlock()
	if cmd.Args[0] == "ps" {
		return compose.Result{Stdout: []byte(strings.Join(f.ps, "\n") + "\n")}, nil
	}
	if f.handle == nil {
		return compose.Result{}, nil
	}
	return f.handle(ctx, cmd)
}

func psLine(id, service, proj, run, role string) string {
	return strings.Join([]string{id, proj + "-" + service + "-1", "running", "Up", proj, service, run, role}, "\t")
}

type freezableAlive struct {
	inner  collect.FileAlive
	mu     sync.Mutex
	frozen bool
}

func (a *freezableAlive) Load() (time.Time, bool, error) { return a.inner.Load() }

func (a *freezableAlive) Save(at time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.frozen {
		return nil
	}
	return a.inner.Save(at)
}

func (a *freezableAlive) Clear() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.frozen {
		return nil
	}
	return a.inner.Clear()
}

func (a *freezableAlive) freeze() {
	a.mu.Lock()
	a.frozen = true
	a.mu.Unlock()
}

package collect

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports"
)

const SupervisorCollector = "supervisor"

var ErrUnknownRun = errors.New("the run is not registered with the collector supervisor")

type Hooks interface {
	Arm(ctx context.Context, runID string, cause lifecycle.HookCause) error
	Drain(ctx context.Context, runID string, cause lifecycle.HookCause) error
}

func Fire(ctx context.Context, h Hooks, runID string, hook lifecycle.Hook) error {
	switch hook.Action {
	case lifecycle.HookArm:
		return h.Arm(ctx, runID, hook.Cause)
	case lifecycle.HookDrain:
		return h.Drain(ctx, runID, hook.Cause)
	}
	return nil
}

type Runner func(ctx context.Context, spec worker.Spec) (worker.Outcome, error)

type Emitter interface {
	Append(sig contract.Signal) (int64, error)
}

type Observer interface {
	Name() string
	Run(ctx context.Context) error
}

type Collector struct {
	ID       string
	Code     string
	Interval time.Duration
	Allowed  []string
}

type RunConfig struct {
	RunID      string
	Collectors []Collector
	Ports      func(collector string) map[string]ports.Handler
	Signals    Emitter
	Alive      Alive
	Observers  []Observer
	Mask       ports.Mask
}

type Options struct {
	Runner       Runner
	Command      []string
	MaxBudget    time.Duration
	DrainWait    time.Duration
	ReportBudget time.Duration
	FailAfter    int
	BackoffMax   time.Duration
	Heartbeat    time.Duration
	Now          func() time.Time
	Sleep        func(ctx context.Context, d time.Duration) bool
	Log          *slog.Logger
}

type Supervisor struct {
	opts Options
	mu   sync.Mutex
	runs map[string]*run
}

type generation struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type run struct {
	cfg    RunConfig
	mu     sync.Mutex
	gen    *generation
	tracks map[string]*track
	ledger ledger
}

func New(opts Options) *Supervisor {
	if opts.Runner == nil {
		opts.Runner = worker.Run
	}
	if opts.MaxBudget <= 0 {
		opts.MaxBudget = worker.DefaultDeadline
	}
	if opts.DrainWait <= 0 {
		opts.DrainWait = 5 * time.Second
	}
	if opts.ReportBudget <= 0 {
		opts.ReportBudget = time.Minute
	}
	if opts.FailAfter <= 0 {
		opts.FailAfter = 3
	}
	if opts.BackoffMax <= 0 {
		opts.BackoffMax = 5 * time.Minute
	}
	if opts.Heartbeat <= 0 {
		opts.Heartbeat = 5 * time.Second
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Sleep == nil {
		opts.Sleep = sleep
	}
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	return &Supervisor{opts: opts, runs: map[string]*run{}}
}

func (s *Supervisor) Register(cfg RunConfig) error {
	if cfg.RunID == "" || cfg.Ports == nil {
		return errors.New("a run config needs a run id and a port factory")
	}
	if cfg.Mask == nil {
		cfg.Mask = ports.Identity
	}
	r := &run{cfg: cfg, tracks: map[string]*track{}}
	for _, c := range cfg.Collectors {
		if _, dup := r.tracks[c.ID]; dup {
			return fmt.Errorf("collector %q is declared twice", c.ID)
		}
		r.tracks[c.ID] = &track{interval: c.Interval, state: StateStopped}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old := s.runs[cfg.RunID]; old != nil && old.armed() {
		return fmt.Errorf("run %s is armed; drain it before registering again", cfg.RunID)
	}
	s.runs[cfg.RunID] = r
	return nil
}

func (s *Supervisor) Forget(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runs, runID)
}

func (s *Supervisor) get(runID string) *run {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[runID]
}

func (r *run) armed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gen != nil
}

func (s *Supervisor) Arm(_ context.Context, runID string, cause lifecycle.HookCause) error {
	r := s.get(runID)
	if r == nil {
		return ErrUnknownRun
	}
	r.mu.Lock()
	if r.gen != nil {
		r.mu.Unlock()
		return nil
	}
	g := &generation{}
	g.ctx, g.cancel = context.WithCancel(context.Background())
	r.gen = g
	for _, t := range r.tracks {
		t.arm()
	}
	r.mu.Unlock()
	if cause == lifecycle.CauseReconciled {
		s.gap(r)
	}
	s.heartbeat(g, r)
	for _, c := range r.cfg.Collectors {
		if c.Interval > 0 {
			g.wg.Go(func() { s.loop(g.ctx, r, c) })
		}
	}
	for _, o := range r.cfg.Observers {
		g.wg.Go(func() { s.observe(g.ctx, r, o) })
	}
	s.opts.Log.Info("collectors armed", "run", runID, "cause", cause)
	return nil
}

func (s *Supervisor) spec(r *run, c Collector, budget time.Duration) worker.Spec {
	return worker.Spec{
		Command: s.opts.Command, Collector: c.ID, Code: c.Code, Allowed: c.Allowed,
		Ports: r.cfg.Ports(c.ID), Mask: r.cfg.Mask, Limits: worker.Limits{Deadline: budget}, Log: s.opts.Log,
	}
}

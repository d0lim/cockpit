package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/d0lim/cockpit/internal/home"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/config"
	"github.com/d0lim/cockpit/internal/hub/lock"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/sse"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ui"
	"github.com/d0lim/cockpit/internal/mask"
)

const (
	DefaultSync       = time.Second
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 10 * time.Second
)

type Scheduler interface {
	api.Scheduler
	Tick()
	Reconcile() bool
	Dispose()
	Idle()
}

type BuildInput struct {
	Home    string
	Config  config.Config
	HubPort int
	Store   *store.Store
	Bus     *sse.Bus
	Streams *Streams
	Log     *slog.Logger
}

type Built struct {
	Scheduler  Scheduler
	Signals    sse.Signals
	Collectors api.Collectors
	Services   func(m store.Meta) []api.ServicePort
	Suites     func(ctx context.Context, worktree string) ([]string, error)
	Mask       func(runID string) *mask.Masker
	Close      func()
}

type Builder func(in BuildInput) (Built, error)

type Options struct {
	Getenv func(string) string
	Build  Builder
	Log    *slog.Logger
	Sync   time.Duration
	Ping   time.Duration
	Poll   time.Duration
	Ready  func(addr string)
	Now    func() time.Time
}

func Run(ctx context.Context, o Options) error {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	dir, err := home.Dir(o.Getenv)
	if err != nil {
		return err
	}
	if err := store.RefuseLegacyLayout(dir); err != nil {
		return err
	}
	addr, port, err := lock.ListenAddr(o.Getenv)
	if err != nil {
		return err
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}
	if _, err := lock.Acquire(dir, port); err != nil {
		return err
	}
	defer lock.Release(dir)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", addr, err)
	}
	defer ln.Close()
	st := store.Open(dir)
	bus := sse.New(strconv.FormatInt(o.Now().UnixMilli(), 10), sse.DefaultCapacity, o.Now)
	var built Built
	streams := NewStreams(st, func(id string) *mask.Masker { return built.Mask(id) })
	built, err = o.Build(BuildInput{Home: dir, Config: cfg, HubPort: port, Store: st, Bus: bus, Streams: streams, Log: o.Log})
	if err != nil {
		return err
	}
	sched := built.Scheduler
	defer func() {
		sched.Dispose()
		sched.Idle()
		if built.Close != nil {
			built.Close()
		}
		streams.CloseAll()
	}()
	if built.Mask == nil {
		built.Mask = PatternMasker
	}
	if built.Signals == nil {
		built.Signals = streams
	}
	runs := newRunsSync(sched, bus, o.Log)
	handler := api.Handler(api.Deps{
		Port: port, Store: st, Scheduler: sched, Signals: built.Signals, Collectors: built.Collectors,
		Services: built.Services, Suites: built.Suites, Mask: built.Mask, Changed: runs.kick,
		Stream: &sse.Stream{Bus: bus, Signals: built.Signals, Ping: o.Ping, Poll: o.Poll}, Screen: ui.Files(),
	})
	base, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: readHeaderTimeout, BaseContext: func(net.Listener) context.Context { return base }}
	sched.Reconcile()
	sched.Tick()
	go runs.loop(ctx, orDefault(o.Sync, DefaultSync))
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	o.Log.Info("hub listening", "addr", ln.Addr().String(), "home", dir)
	if o.Ready != nil {
		o.Ready(ln.Addr().String())
	}
	select {
	case <-ctx.Done():
	case err := <-served:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	cancelBase()
	down, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(down)
}

func orDefault(d, fallback time.Duration) time.Duration {
	if d <= 0 {
		return fallback
	}
	return d
}

var _ Scheduler = (*scheduler.Scheduler)(nil)

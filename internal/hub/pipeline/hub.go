package pipeline

import (
	"context"
	"log/slog"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/config"
	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/hub/exec"
	"github.com/d0lim/cockpit/internal/hub/lease"
	"github.com/d0lim/cockpit/internal/hub/pool"
	"github.com/d0lim/cockpit/internal/hub/procgroup"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/runnerbin"
	"github.com/d0lim/cockpit/internal/version"
)

type Event struct {
	Run  string
	Line string
}

type Options struct {
	Home         string
	Config       config.Config
	HubPort      int
	DockerFor    func(h procgroup.Hooks) compose.Docker
	Probe        reconcile.Docker
	Ensure       func(ctx context.Context) (runnerbin.Image, error)
	Reporter     envops.Reporter
	Limits       stages.Limits
	PortProbe    lease.Probe
	StopOnDone   bool
	TickInterval time.Duration
	Now          func() time.Time
	BootTime     func() time.Time
	RunnerUser   string
	Events       func(Event)
	Log          *slog.Logger
}

type Hub struct {
	o      Options
	store  *store.Store
	ops    *envops.Ops
	layout lease.Layout
}

func ConfigLimits(cfg config.Config) pool.Limits {
	return pool.Limits{PoolMax: cfg.Limits.KeptPool, QueueMax: cfg.Limits.QueueCap, UpCap: cfg.Limits.RunningCap}
}

func ConfigLayout(cfg config.Config, hubPort int) lease.Layout {
	reserved := make([]lease.Reserved, 0, len(cfg.ReservedPorts))
	for _, r := range cfg.ReservedPorts {
		reserved = append(reserved, lease.Reserved{Name: r.Name, From: r.From, To: r.To})
	}
	return lease.Layout{Base: cfg.Limits.PortBase, Stride: config.PortWidth, Slots: cfg.Limits.Slots, HubPort: hubPort, Reserved: reserved}
}

func New(o Options) (*scheduler.Scheduler, *Hub, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.BootTime == nil {
		boot := o.Now()
		o.BootTime = func() time.Time { return boot }
	}
	if o.PortProbe == nil {
		o.PortProbe = lease.ProbeLoopback
	}
	if o.RunnerUser == "" {
		o.RunnerUser = stages.HostRunnerUser()
	}
	if o.Reporter == nil {
		o.Reporter = DefaultReport
	}
	if o.Events == nil {
		o.Events = func(Event) {}
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	layout := ConfigLayout(o.Config, o.HubPort)
	if err := layout.Validate(); err != nil {
		return nil, nil, err
	}
	st := store.Open(o.Home)
	ops := &envops.Ops{Store: st, DockerFor: o.DockerFor, Ensure: o.Ensure, Layout: layout, Probe: o.PortProbe, Limits: o.Limits, Reporter: o.Reporter, Now: o.Now}
	h := &Hub{o: o, store: st, ops: ops, layout: layout}
	deps := scheduler.Deps{
		Deps: ticker.Deps{
			Store: st, Docker: o.Probe, Env: ops, Lease: h.Lease, Pipeline: h.Pipeline,
			Limits: ConfigLimits(o.Config), StopOnDone: o.StopOnDone, TickInterval: o.TickInterval,
			Now: o.Now, BootTime: o.BootTime, Served: compose.ServedHostAPIs, CockpitVersion: version.String(), Log: o.Log,
		},
		Accept: h.Accept,
		Exec:   &exec.Execer{Ops: ops, Now: o.Now},
	}
	return scheduler.New(deps), h, nil
}

func (h *Hub) Ops() *envops.Ops { return h.ops }

func (h *Hub) Store() *store.Store { return h.store }

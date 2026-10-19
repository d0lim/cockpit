package ticker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/d0lim/cockpit/internal/hub/pool"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/store"
)

type Env interface {
	reconcile.Env
	Stop(ctx context.Context, id string) Result
	Destroy(ctx context.Context, id string) Result
	Resume(ctx context.Context, id string) Result
}

type Deps struct {
	Store          *store.Store
	Docker         reconcile.Docker
	Env            Env
	Lease          func(leases store.Leases, id string) (int, error)
	Pipeline       func(ctx context.Context, id string) error
	Limits         pool.Limits
	StopOnDone     bool
	TickInterval   time.Duration
	Now            func() time.Time
	BootTime       func() time.Time
	Served         []int
	CockpitVersion string
	Log            *slog.Logger
}

func (d Deps) reconcileDeps() reconcile.Deps {
	return reconcile.Deps{Store: d.Store, Docker: d.Docker, Env: d.Env, Now: d.Now, BootTime: d.BootTime, Served: d.Served, CockpitVersion: d.CockpitVersion}
}

type Active struct {
	ID     string
	cancel context.CancelFunc
	Done   chan struct{}
}

type Core struct {
	d          Deps
	base       context.Context
	cancelBase context.CancelFunc
	mu         sync.Mutex
	cond       *sync.Cond
	pending    int
	active     *Active
	ops        map[string]int
	scan       reconcile.Scan
	managed    *reconcile.Managed
	dockerDown bool
	reconciled bool
	tickErr    string
	lastRedo   map[string]time.Time
	ticking    chan struct{}
	again      bool
	timerStop  chan struct{}
	disposed   bool
}

func New(d Deps) *Core {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	base, cancel := context.WithCancel(context.Background())
	c := &Core{d: d, base: base, cancelBase: cancel, ops: map[string]int{}, managed: reconcile.NewManaged(), lastRedo: map[string]time.Time{}, scan: reconcile.Scan{Orphans: []reconcile.Orphan{}}}
	c.cond = sync.NewCond(&c.mu)
	return c
}

func (c *Core) Deps() Deps { return c.d }

func (c *Core) Context() context.Context { return c.base }

func (c *Core) begin() {
	c.mu.Lock()
	c.pending++
	c.mu.Unlock()
}

func (c *Core) end() {
	c.mu.Lock()
	c.pending--
	c.cond.Broadcast()
	c.mu.Unlock()
}

func (c *Core) track(fn func()) {
	c.begin()
	go func() {
		defer c.end()
		fn()
	}()
}

func (c *Core) goTick() { c.track(c.Tick) }

func (c *Core) Kick() { c.goTick() }

const ReasonBusy = "an operation is in progress on this run"

func (c *Core) claim(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ops[id] > 0 {
		return false
	}
	c.ops[id]++
	c.pending++
	return true
}

func (c *Core) refuseBusy(id string) Result {
	m, _ := c.d.Store.ReadMeta(id)
	return Conflict(ReasonBusy, m)
}

func (c *Core) release(id string, tick bool) {
	c.mu.Lock()
	if c.ops[id]--; c.ops[id] <= 0 {
		delete(c.ops, id)
	}
	c.pending--
	c.cond.Broadcast()
	c.mu.Unlock()
	if tick {
		c.goTick()
	}
}

func (c *Core) RunOp(id string, fn func() Result, tick bool) Result {
	if !c.claim(id) {
		return c.refuseBusy(id)
	}
	defer c.release(id, tick)
	return fn()
}

func (c *Core) Operate(id string, apply func() (store.Meta, error), op func() Result) Result {
	if !c.claim(id) {
		return c.refuseBusy(id)
	}
	current, err := apply()
	if err != nil {
		c.release(id, false)
		return c.applyFailure(err, current)
	}
	defer c.release(id, true)
	return op()
}

func (c *Core) Busy(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.ops[id]
	return ok
}

func (c *Core) ActiveRun() *Active {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active
}

func (c *Core) Abort(id string) bool {
	c.mu.Lock()
	a := c.active
	c.mu.Unlock()
	if a == nil || a.ID != id {
		return false
	}
	a.cancel()
	return true
}

func (c *Core) Orphans() []reconcile.Orphan {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scan.Orphans
}

func (c *Core) DockerDown() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dockerDown
}

func (c *Core) TickError() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tickErr
}

func (c *Core) View(runs []store.Meta) pool.View {
	c.mu.Lock()
	defer c.mu.Unlock()
	return pool.View{Runs: runs, ForeignUp: c.scan.ForeignUp, OrphansUp: c.scan.OrphansUp(), Cap: c.d.Limits.UpCap, PoolMax: c.d.Limits.PoolMax}
}

func (c *Core) HasActive(v pool.View) bool {
	if c.ActiveRun() != nil {
		return true
	}
	for _, m := range v.Runs {
		if m.Active() {
			return true
		}
	}
	return false
}

func (c *Core) ActiveShare(v pool.View) bool {
	if c.HasActive(v) {
		return true
	}
	st, err := c.d.Store.CurrentState()
	return err == nil && !c.DockerDown() && len(st.Queue) > 0 && pool.StartBlock(v, false) == ""
}

func (c *Core) RefreshProjects(ctx context.Context) error {
	list, err := c.d.Store.ListRuns()
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, m := range list.Runs {
		known[m.ID] = true
	}
	scan, err := reconcile.ScanProjects(ctx, c.d.Docker, known, c.managed, nil)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.scan = scan
	c.mu.Unlock()
	return nil
}

func (c *Core) Idle() {
	c.mu.Lock()
	for c.pending > 0 {
		c.cond.Wait()
	}
	c.mu.Unlock()
}

func (c *Core) Dispose() {
	c.mu.Lock()
	c.disposed = true
	if c.timerStop != nil {
		close(c.timerStop)
		c.timerStop = nil
	}
	c.mu.Unlock()
	c.cancelBase()
}

func (c *Core) ensureTimer() {
	st, err := c.d.Store.CurrentState()
	c.mu.Lock()
	defer c.mu.Unlock()
	needed := !c.disposed && (c.dockerDown || (err == nil && len(st.Queue) > 0))
	switch {
	case needed && c.timerStop == nil:
		stop := make(chan struct{})
		c.timerStop = stop
		go func() {
			t := time.NewTicker(c.d.TickInterval)
			defer t.Stop()
			for {
				select {
				case <-stop:
					return
				case <-t.C:
					c.goTick()
				}
			}
		}()
	case !needed && c.timerStop != nil:
		close(c.timerStop)
		c.timerStop = nil
	}
}

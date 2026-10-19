package ticker

import (
	"context"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/pool"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/store"
)

func (c *Core) Tick() {
	c.mu.Lock()
	if c.disposed {
		c.mu.Unlock()
		return
	}
	c.pending++
	if c.ticking != nil {
		c.again = true
		ch := c.ticking
		c.mu.Unlock()
		<-ch
		c.end()
		return
	}
	ch := make(chan struct{})
	c.ticking = ch
	c.mu.Unlock()
	for {
		c.mu.Lock()
		c.again = false
		c.mu.Unlock()
		err := c.tickOnce()
		c.mu.Lock()
		c.tickErr = ""
		if err != nil {
			c.tickErr = err.Error()
			c.d.Log.Warn("tick failed", "err", err)
		}
		if !c.again {
			c.ticking = nil
			close(ch)
			c.mu.Unlock()
			break
		}
		c.mu.Unlock()
	}
	c.ensureTimer()
	c.end()
}

func (c *Core) Reconcile() bool {
	c.mu.Lock()
	c.reconciled = false
	c.mu.Unlock()
	c.Tick()
	return c.DockerDown()
}

func (c *Core) markWaiting(reason lifecycle.WaitReason) error {
	st, err := c.d.Store.CurrentState()
	if err != nil {
		return err
	}
	for _, id := range st.Queue {
		m, err := c.d.Store.ReadMeta(id)
		if err != nil || m == nil || m.Phase != lifecycle.Queued || m.WaitReason == reason {
			continue
		}
		if _, err := c.d.Store.PatchMeta(id, func(x *store.Meta) { x.WaitReason = reason }); err != nil {
			return err
		}
	}
	return nil
}

func (c *Core) skipSet() map[string]bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	skip := map[string]bool{}
	for id := range c.ops {
		skip[id] = true
	}
	if c.active != nil {
		skip[c.active.ID] = true
	}
	return skip
}

func (c *Core) reconcileAll(ctx context.Context) error {
	hooks := reconcile.Hooks{
		Skip:    c.skipSet(),
		Managed: c.managed,
		RunOp: func(id string, fn func() error) error {
			var err error
			c.RunOp(id, func() Result { err = fn(); return Result{} }, false)
			return err
		},
	}
	scan, err := reconcile.Run(ctx, c.d.reconcileDeps(), hooks)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.scan, c.reconciled, c.dockerDown = scan, true, false
	c.mu.Unlock()
	return nil
}

func (c *Core) redoStalled(runs []store.Meta) {
	now := c.d.Now()
	for _, m := range runs {
		if !m.InProgress() || c.Busy(m.ID) {
			continue
		}
		c.mu.Lock()
		skip := (c.active != nil && c.active.ID == m.ID) || now.Sub(c.lastRedo[m.ID]) < c.d.TickInterval
		if !skip {
			c.lastRedo[m.ID] = now
		}
		c.mu.Unlock()
		if skip {
			continue
		}
		id := m.ID
		c.track(func() {
			c.RunOp(id, func() Result {
				if err := c.d.Env.Redo(c.base, id); err != nil {
					return Failed(500, CodeOpFailed, err.Error(), nil)
				}
				return Result{}
			}, true)
		})
	}
}

func (c *Core) tickOnce() error {
	ctx := c.base
	if !c.d.Docker.Info(ctx) {
		c.mu.Lock()
		c.dockerDown = true
		c.mu.Unlock()
		return c.markWaiting(lifecycle.WaitDockerUnavailable)
	}
	c.mu.Lock()
	full := c.dockerDown || !c.reconciled
	c.mu.Unlock()
	if full {
		if err := c.reconcileAll(ctx); err != nil {
			return err
		}
	} else if err := c.RefreshProjects(ctx); err != nil {
		c.d.Log.Warn("project scan failed", "err", err)
	}
	list, err := c.d.Store.ListRuns()
	if err != nil {
		return err
	}
	runs := list.Runs
	c.redoStalled(runs)
	for victim := pool.Victim(runs, c.d.Limits.PoolMax); victim != nil; victim = pool.Victim(runs, c.d.Limits.PoolMax) {
		destroyed := c.DestroyRun(victim.ID, lifecycle.DestroyPool)
		if list, err = c.d.Store.ListRuns(); err != nil {
			return err
		}
		runs = list.Runs
		if !destroyed.OK() {
			break
		}
	}
	before := c.View(runs)
	victims := pool.CapVictims(before, c.ActiveShare(before))
	for _, v := range victims {
		c.StopRun(v.ID, lifecycle.StopCap)
	}
	if len(victims) > 0 {
		if list, err = c.d.Store.ListRuns(); err != nil {
			return err
		}
		runs = list.Runs
	}
	view := c.View(runs)
	if block := pool.StartBlock(view, c.HasActive(view)); block != lifecycle.WaitNone {
		return c.markWaiting(block)
	}
	return c.startHead()
}

func (c *Core) startHead() error {
	st, err := c.d.Store.CurrentState()
	if err != nil || len(st.Queue) == 0 {
		return err
	}
	head := st.Queue[0]
	slot, err := c.d.Lease(st.Leases, head)
	if err != nil {
		return c.markWaiting(lifecycle.WaitNoPortBlock)
	}
	now, err := c.d.Store.CurrentState()
	if err != nil {
		return err
	}
	taken := false
	for _, s := range now.Leases {
		taken = taken || s == slot
	}
	if c.ActiveRun() != nil || len(now.Queue) == 0 || now.Queue[0] != head || taken {
		return nil
	}
	if _, err := c.d.Store.ApplyEvent(head, lifecycle.TickStart, lifecycle.Detail{}, func(m *store.Meta) {
		m.StartedAt = store.Timestamp(c.d.Now())
		m.Slot = &slot
		m.WaitReason = lifecycle.WaitNone
	}); err != nil {
		return nil
	}
	if _, err := c.d.Store.UpdateState(func(s store.HubState) store.HubState {
		next := s.Without(head)
		next.Leases[head] = slot
		return next
	}); err != nil {
		return err
	}
	c.launch(head)
	return c.markWaiting(lifecycle.WaitActiveRun)
}

func (c *Core) launch(id string) {
	ctx, cancel := context.WithCancel(c.base)
	a := &Active{ID: id, cancel: cancel, Done: make(chan struct{})}
	c.mu.Lock()
	c.active = a
	c.mu.Unlock()
	c.track(func() {
		defer close(a.Done)
		if err := c.d.Pipeline(ctx, id); err != nil {
			closed, aerr := c.d.Store.ApplyEvent(id, lifecycle.Fail, lifecycle.Detail{}, func(m *store.Meta) {
				m.Notice = "the pipeline stopped: " + c.d.Env.Mask(id, err.Error())
			})
			if aerr == nil && closed.Phase == lifecycle.Done && closed.Env == lifecycle.EnvAbsent {
				c.d.Env.CloseWithoutEnv(c.base, id, false)
			}
		}
		c.mu.Lock()
		if c.active == a {
			c.active = nil
		}
		c.mu.Unlock()
		cancel()
		c.stopFinished(id)
		c.goTick()
	})
}

func (c *Core) stopFinished(id string) {
	if !c.d.StopOnDone {
		return
	}
	if m, err := c.d.Store.ReadMeta(id); err == nil && m != nil && pool.Stoppable(*m) {
		c.StopRun(id, lifecycle.StopDone)
	}
}

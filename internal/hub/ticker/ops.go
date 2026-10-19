package ticker

import (
	"errors"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
)

func (c *Core) StopRun(id string, reason lifecycle.StopReason) Result {
	current, err := c.d.Store.ApplyEvent(id, lifecycle.Stop, lifecycle.Detail{}, func(m *store.Meta) { m.StopReason = reason })
	if err != nil {
		return c.applyFailure(err, current)
	}
	return c.RunOp(id, func() Result { return c.d.Env.Stop(c.base, id) }, true)
}

func (c *Core) DestroyRun(id string, reason lifecycle.DestroyReason) Result {
	before, err := c.d.Store.MustRead(id)
	if err != nil {
		return Failed(404, CodeNotFound, err.Error(), nil)
	}
	current, err := c.d.Store.ApplyEvent(id, lifecycle.Destroy, lifecycle.Detail{}, func(m *store.Meta) {
		m.DestroyReason = reason
		m.WaitReason = lifecycle.WaitNone
	})
	if err != nil {
		return c.applyFailure(err, current)
	}
	if before.Phase == lifecycle.Queued {
		if _, err := c.d.Store.UpdateState(func(s store.HubState) store.HubState { return s.Without(id) }); err != nil {
			return Failed(500, CodeOpFailed, err.Error(), &current)
		}
		run, err := c.d.Store.PatchMeta(id, func(m *store.Meta) { m.FinishedAt = store.Timestamp(c.d.Now()) })
		if err != nil {
			return Failed(500, CodeOpFailed, err.Error(), &current)
		}
		c.goTick()
		return Succeeded(run)
	}
	var running *Active
	if a := c.ActiveRun(); before.Active() && a != nil && a.ID == id {
		running = a
		a.cancel()
	}
	return c.RunOp(id, func() Result {
		if running != nil {
			<-running.Done
		}
		return c.d.Env.Destroy(c.base, id)
	}, true)
}

func (c *Core) applyFailure(err error, current store.Meta) Result {
	var conflict *lifecycle.Conflict
	if errors.As(err, &conflict) {
		return Conflict(err.Error(), &current)
	}
	return Failed(500, CodeOpFailed, err.Error(), nil)
}

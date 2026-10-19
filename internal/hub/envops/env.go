package envops

import (
	"context"
	"errors"
	"fmt"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/lease"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/runfiles"
	"github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
)

func (o *Ops) finish(st *store.Store, id string, e lifecycle.Event, extra func(*store.Meta)) ticker.Result {
	m, err := st.ApplyEvent(id, e, lifecycle.Detail{}, func(x *store.Meta) {
		x.Pgid = nil
		if extra != nil {
			extra(x)
		}
	})
	if err != nil {
		return ticker.Conflict(err.Error(), &m)
	}
	return ticker.Succeeded(m)
}

func (o *Ops) stuck(r *Run, verb string, err error) ticker.Result {
	reason := fmt.Sprintf("%s failed: %s; the next tick tries again", verb, r.Mask(err.Error()))
	m, _ := r.Store.PatchMeta(r.Meta.ID, func(x *store.Meta) { x.Notice = reason; x.Pgid = nil })
	return ticker.Failed(500, ticker.CodeOpFailed, reason, &m)
}

func (o *Ops) Stop(ctx context.Context, id string) ticker.Result {
	r, err := o.Open(id)
	if err != nil {
		return ticker.Failed(404, ticker.CodeNotFound, err.Error(), nil)
	}
	defer r.Close()
	if err := r.Project.Stop(ctx); err != nil {
		return o.stuck(r, "stop", err)
	}
	return o.finish(r.Store, id, lifecycle.Stopped, nil)
}

func (o *Ops) Destroy(ctx context.Context, id string) ticker.Result {
	r, err := o.Open(id)
	if err != nil {
		return ticker.Failed(404, ticker.CodeNotFound, err.Error(), nil)
	}
	defer r.Close()
	if err := r.Project.Down(ctx); err != nil {
		return o.stuck(r, "destroy", err)
	}
	removeEnvDirs(r.Layout.Root)
	o.maskers.Delete(id)
	if err := o.releaseLease(id); err != nil {
		return o.stuck(r, "destroy", err)
	}
	return o.finish(r.Store, id, lifecycle.Destroyed, nil)
}

func (o *Ops) giveUp(r *Run, status int, code ticker.Code, reason string) ticker.Result {
	m, _ := r.Store.ApplyEvent(r.Meta.ID, lifecycle.ResumeFailed, lifecycle.Detail{}, func(x *store.Meta) { x.Notice = reason; x.Pgid = nil })
	return ticker.Failed(status, code, reason, &m)
}

func (o *Ops) RunPorts(r *Run) map[string]int {
	ports := map[string]int{}
	if r.Meta.Slot == nil || r.Decl == nil {
		return ports
	}
	base := o.Layout.Base + o.Layout.Stride**r.Meta.Slot
	for _, p := range r.Decl.Ports {
		ports[string(p.Name)] = compose.HostPort(base, p)
	}
	return ports
}

func (o *Ops) Resume(ctx context.Context, id string) ticker.Result {
	r, err := o.Open(id)
	if err != nil {
		return ticker.Failed(404, ticker.CodeNotFound, err.Error(), nil)
	}
	defer r.Close()
	if r.Decl == nil || r.Meta.Slot == nil {
		return o.giveUp(r, 500, ticker.CodeOpFailed, "resume failed: the run has no declaration copy or port block record")
	}
	if blocked := lease.BlockedPorts(o.RunPorts(r), o.Probe); len(blocked) > 0 {
		return o.giveUp(r, 409, ticker.CodeConflictPort, fmt.Sprintf("resume failed: ports are taken (%v); the block is not leased again", blocked))
	}
	if err := o.VerifyPin(ctx, r); err != nil {
		return o.giveUp(r, 500, ticker.CodeOpFailed, "resume failed: "+r.Mask(err.Error()))
	}
	err = nil
	if r.Meta.Recreate {
		err = r.Project.UpRecreate(ctx)
	} else if err = r.Project.Start(ctx, o.Limits.Boot, o.Limits.Poll); err != nil {
		var missing *compose.EnvMissing
		if errors.As(err, &missing) {
			err = r.Project.UpRecreate(ctx)
		}
	}
	if err == nil {
		err = stages.AwaitBoot(ctx, r.Project, r.Decl.Project.ExpectedServices, o.Limits.Boot, o.Limits.Poll)
	}
	if err != nil {
		r.Project.Stop(context.WithoutCancel(ctx))
		return o.giveUp(r, 500, ticker.CodeOpFailed, "resume failed: "+r.Mask(err.Error()))
	}
	return o.finish(r.Store, id, lifecycle.Resumed, func(x *store.Meta) { x.LastUpAt = store.Timestamp(o.Now()); x.Recreate = false })
}

func (o *Ops) Redo(ctx context.Context, id string) error {
	m, err := o.Store.MustRead(id)
	if err != nil {
		return err
	}
	var res ticker.Result
	switch m.Phase {
	case lifecycle.Stopping:
		res = o.Stop(ctx, id)
	case lifecycle.Resuming:
		res = o.Resume(ctx, id)
	case lifecycle.Destroying:
		res = o.Destroy(ctx, id)
	default:
		return fmt.Errorf("%s is not an operation in progress", m.Phase)
	}
	if res.Fail != nil {
		return res.Fail
	}
	return nil
}

func (o *Ops) HasRunnerResult(id string) bool {
	r, err := o.Open(id)
	if err != nil || r.Root == nil {
		return false
	}
	defer r.Close()
	_, err = runfiles.Result(r.Root)
	return err == nil
}

func (o *Ops) Report(ctx context.Context, m store.Meta) (reconcile.ReportOutcome, error) {
	r, err := o.Open(m.ID)
	if err != nil {
		return reconcile.ReportOutcome{}, err
	}
	defer r.Close()
	out, _, err := o.Reporter(ctx, ReportInput{Meta: m, Store: r.Store, RunDir: r.Layout.Root, Decl: r.Decl, Ports: o.RunPorts(r), Masker: r.Masker, Now: o.Now()})
	return out, err
}

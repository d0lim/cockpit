package envops

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/signal"
)

var ErrCollectionOff = errors.New("collection is off on this hub")

type collected struct {
	mu      sync.Mutex
	defs    []compose.CollectorDef
	stream  *signal.Stream
	closers []func() error
	live    bool
}

func (o *Ops) log() *slog.Logger {
	if o.Log == nil {
		return slog.Default()
	}
	return o.Log
}

func (o *Ops) currentMasker(id string) *mask.Masker {
	if cached, ok := o.maskers.Load(id); ok {
		return cached.(*mask.Masker)
	}
	m, _ := mask.New(mask.Options{Mode: mask.Hub})
	return m
}

func (o *Ops) entry(id string) *collected {
	v, _ := o.runs.LoadOrStore(id, &collected{})
	return v.(*collected)
}

func (o *Ops) Discover(ctx context.Context, id string, decl compose.Loaded, scriptsDir string) error {
	if o.Collect == nil {
		return nil
	}
	defs, err := compose.CollectorDefs(ctx, decl.Declaration, scriptsDir)
	if err != nil {
		return err
	}
	c := o.entry(id)
	c.mu.Lock()
	c.defs = defs
	c.mu.Unlock()
	return nil
}

func (o *Ops) Stream(id string) *signal.Stream {
	v, ok := o.runs.Load(id)
	if !ok {
		return nil
	}
	c := v.(*collected)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stream
}

func (o *Ops) Transition(t store.Transition) {
	if o.Collect == nil {
		return
	}
	id := t.After.ID
	before := t.Before
	hook := lifecycle.HookFor(&before, t.Event, t.Detail, t.After.State)
	ctx := context.Background()
	if hook.Action == lifecycle.HookArm {
		if err := o.register(ctx, id); err != nil {
			o.log().Warn("collectors cannot be registered", "run", id, "err", o.Mask(id, err.Error()))
			return
		}
	}
	if err := collect.Fire(ctx, o.Collect, id, hook); err != nil {
		o.log().Warn("collector hook did not finish cleanly", "run", id, "action", hook.Action, "cause", hook.Cause, "err", o.Mask(id, err.Error()))
	}
	if ends(t.After) {
		o.forget(id)
	}
}

func ends(m store.Meta) bool {
	return m.Mark == lifecycle.Lost || m.Phase == lifecycle.Done && (m.Env == lifecycle.EnvAbsent || m.Env == lifecycle.EnvDestroyed)
}

func (o *Ops) forget(id string) {
	o.Collect.Forget(id)
	v, ok := o.runs.LoadAndDelete(id)
	if !ok {
		return
	}
	c := v.(*collected)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, close := range c.closers {
		close()
	}
	if c.stream != nil {
		c.stream.Close()
	}
}

func (o *Ops) CollectReport(ctx context.Context, id string, env lifecycle.Env) (*collect.ReportResult, error) {
	if o.Collect == nil {
		return nil, ErrCollectionOff
	}
	if err := o.register(ctx, id); err != nil {
		return nil, err
	}
	res, err := o.Collect.Report(ctx, id, env)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (o *Ops) Close() {
	o.runs.Range(func(k, _ any) bool {
		id := k.(string)
		o.Collect.Drain(context.Background(), id, lifecycle.CauseAbort)
		o.forget(id)
		return true
	})
}

func (o *Ops) AddCollected(ctx context.Context, in *ReportInput) {
	if o.Collect == nil {
		return
	}
	in.Collected, in.CollectErr = o.CollectReport(ctx, in.Meta.ID, in.Meta.Env)
	if s := o.Stream(in.Meta.ID); s != nil {
		in.Signals = s
	}
}

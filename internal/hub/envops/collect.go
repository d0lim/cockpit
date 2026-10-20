package envops

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/signal"
)

var ErrCollectionOff = errors.New("collection is off on this hub")

type collected struct {
	mu      sync.Mutex
	defs    []compose.CollectorDef
	closers []func() error
	live    bool
}

func (o *Ops) log() *slog.Logger {
	if o.Log == nil {
		return slog.Default()
	}
	return o.Log
}

func (o *Ops) MaskerFor(id string) *mask.Masker {
	if cached, ok := o.maskers.Load(id); ok {
		return cached.(*mask.Masker)
	}
	if r, err := o.Open(id); err == nil {
		r.Close()
		return r.Masker
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
	if o.Streams == nil {
		return nil
	}
	s, err := o.Streams.Get(id)
	if err != nil {
		o.log().Warn("the signal stream cannot be opened", "run", id, "err", o.Mask(id, err.Error()))
		return nil
	}
	return s
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
	if hook.Action == lifecycle.HookDrain && hook.Cause != lifecycle.CauseReporting {
		o.release(id)
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
	v.(*collected).close()
	if o.Streams != nil {
		o.Streams.Close(id)
	}
}

func (o *Ops) release(id string) {
	if v, ok := o.runs.Load(id); ok {
		v.(*collected).close()
	}
}

func (c *collected) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, close := range c.closers {
		close()
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
	if o.Collect == nil {
		return
	}
	o.runs.Range(func(k, _ any) bool {
		id := k.(string)
		o.Collect.Drain(context.Background(), id, lifecycle.CauseAbort)
		o.forget(id)
		return true
	})
}

func (o *Ops) AddCollected(ctx context.Context, in *ReportInput) {
	if o.Collect != nil {
		in.Collected, in.CollectErr = o.CollectReport(ctx, in.Meta.ID, in.Meta.Env)
	}
	if s := o.Stream(in.Meta.ID); s != nil {
		in.Signals, in.Stream = s, s
	}
}

func (o *Ops) AppendSpans(id string, spans []contract.Signal) {
	s := o.Stream(id)
	if s == nil {
		return
	}
	for _, sig := range spans {
		if _, err := s.Append(sig); err != nil {
			o.log().Warn("a runner span was not recorded", "run", id, "span", sig.ID, "err", o.Mask(id, err.Error()))
			return
		}
	}
}

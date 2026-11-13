package envops

import (
	"context"
	"time"

	"github.com/d0lim/cockpit/internal/collect/observer"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/signal"
)

type OTLPDrain struct {
	Waited time.Duration
	Quiet  bool
}

func (o *Ops) otlpOf(id string) *observer.OTLP {
	v, ok := o.runs.Load(id)
	if !ok {
		return nil
	}
	c := v.(*collected)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.otlp
}

func (o *Ops) DrainOTLP(ctx context.Context, id string, quiet, limit time.Duration) (OTLPDrain, bool) {
	obs := o.otlpOf(id)
	if obs == nil || quiet <= 0 {
		return OTLPDrain{}, false
	}
	waited, settled := obs.Settle(ctx, quiet, limit)
	return OTLPDrain{Waited: waited, Quiet: settled}, true
}

func (o *Ops) TraceExec(id, name string) {
	if obs := o.otlpOf(id); obs != nil {
		obs.TraceExec(name)
	}
}

func (o *Ops) ReparentServiceSpans(id, trace string, res contract.Result) {
	s := o.Stream(id)
	if s == nil {
		return
	}
	if _, err := signal.Reparent(s, trace, signal.CallParents(res)); err != nil {
		o.log().Warn("service spans were not moved under their calls", "run", id, "trace", trace, "err", o.Mask(id, err.Error()))
	}
}

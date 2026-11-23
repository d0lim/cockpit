package headless

import (
	"context"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/pipeline"
	"github.com/d0lim/cockpit/internal/signal"
)

func (r *run) share() { r.shared.Store(r.env.Masker) }

func (r *run) maskShared(s string) string { return r.shared.Load().Mask(s) }

func (r *run) observe() error {
	obs, err := envops.NewRunObservers(envops.ObserverInput{
		RunID: r.meta.ID, Decl: r.loaded.Declaration, Docker: r.deps.Docker, Stream: r.stream, Mask: r.maskShared, Now: r.deps.Now, Log: r.logger,
	})
	if err != nil {
		return err
	}
	sup := collect.New(collect.Options{Log: r.logger})
	if err := sup.Register(obs.Config); err != nil {
		return err
	}
	r.observed, r.otlp = sup, obs.OTLP
	return sup.Arm(context.Background(), r.meta.ID, lifecycle.CauseBooted)
}

func (r *run) drainOTLP(ctx context.Context) string {
	limits := r.env.Limits
	d, ok := envops.SettleOTLP(ctx, r.otlp, limits.OTLPQuiet, limits.OTLPDrain)
	return pipeline.OTLPDrainLine(ctx, d, ok, limits.OTLPDrain)
}

func (r *run) quiet() {
	if r.observed == nil {
		return
	}
	cause := lifecycle.CauseAbort
	if r.meta.Phase == lifecycle.Reporting {
		cause = lifecycle.CauseReporting
	}
	r.observed.Drain(context.Background(), r.meta.ID, cause)
}

func (r *run) recorder() *envops.RunWire {
	if !r.opts.Wire {
		return nil
	}
	return &envops.RunWire{
		ID: r.meta.ID, Dir: r.env.Layout.Root, Decl: &r.loaded.Declaration, Docker: r.deps.Docker, Mask: r.maskShared, Log: r.logger,
		Stream: func() *signal.Stream { return r.stream },
	}
}

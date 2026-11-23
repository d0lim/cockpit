package headless

import (
	"context"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/pipeline"
)

func (r *run) share() { r.shared.Store(r.env.Masker) }

func (r *run) maskShared(s string) string { return r.shared.Load().Mask(s) }

func (r *run) observe() {
	obs, err := envops.NewRunObservers(envops.ObserverInput{
		RunID: r.meta.ID, Decl: r.loaded.Declaration, Docker: r.deps.Docker, Stream: r.stream, Mask: r.maskShared, Now: r.deps.Now, Log: r.logger,
	})
	if err == nil {
		sup := collect.New(collect.Options{Log: r.logger})
		if err = sup.Register(obs.Config); err == nil {
			r.observed, r.otlp = sup, obs.OTLP
			err = sup.Arm(context.Background(), r.meta.ID, lifecycle.CauseBooted)
		}
	}
	if err != nil {
		r.logger.Warn("observers cannot be started", "run", r.meta.ID, "err", r.maskShared(err.Error()))
	}
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

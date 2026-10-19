package collect

import (
	"context"
	"time"
)

const observerRetry = time.Second

func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (s *Supervisor) loop(ctx context.Context, r *run, c Collector) {
	for {
		start := s.opts.Now()
		budget := min(c.Interval, s.opts.MaxBudget)
		out, err := s.opts.Runner(ctx, s.spec(r, c, budget))
		if ctx.Err() != nil {
			return
		}
		now := s.opts.Now()
		r.record(c.ID, out, err, now)
		wait := start.Add(c.Interval).Sub(now)
		if b := r.backoff(c.ID, s.opts.FailAfter, s.opts.BackoffMax); b > 0 {
			wait = b
		}
		r.scheduled(c.ID, now.Add(max(wait, 0)))
		if !sleep(ctx, wait) {
			return
		}
	}
}

func (s *Supervisor) observe(ctx context.Context, r *run, o Observer) {
	for {
		err := o.Run(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			r.fold(o.Name(), "observer:"+o.Name(), r.cfg.Mask(err.Error()), s.opts.Now())
		}
		if !sleep(ctx, observerRetry) {
			return
		}
	}
}

func (s *Supervisor) heartbeat(g *generation, r *run) {
	if r.cfg.Alive == nil {
		return
	}
	beat := func() {
		if err := r.cfg.Alive.Save(s.opts.Now()); err != nil {
			s.opts.Log.Warn("cannot record that collectors are alive", "run", r.cfg.RunID, "err", err)
		}
	}
	beat()
	g.wg.Go(func() {
		for sleep(g.ctx, s.opts.Heartbeat) {
			beat()
		}
	})
}

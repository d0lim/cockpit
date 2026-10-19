package collect

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

var ErrDrainTimeout = errors.New("collectors did not stop within the drain wait")

func (s *Supervisor) Drain(ctx context.Context, runID string, cause lifecycle.HookCause) error {
	r := s.get(runID)
	if r == nil {
		return nil
	}
	r.mu.Lock()
	g := r.gen
	r.gen = nil
	for _, t := range r.tracks {
		t.state = StateStopped
	}
	r.mu.Unlock()
	if g == nil {
		return nil
	}
	g.cancel()
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()
	timer := time.NewTimer(s.opts.DrainWait)
	defer timer.Stop()
	var err error
	select {
	case <-done:
	case <-timer.C:
		err = ErrDrainTimeout
	case <-ctx.Done():
		err = ctx.Err()
	}
	if r.cfg.Alive != nil {
		if cerr := r.cfg.Alive.Clear(); cerr != nil {
			s.opts.Log.Warn("cannot clear the collector heartbeat", "run", runID, "err", cerr)
		}
	}
	if err != nil {
		s.opts.Log.Warn("collector drain did not finish cleanly", "run", runID, "cause", cause, "err", err)
	} else {
		s.opts.Log.Info("collectors drained", "run", runID, "cause", cause)
	}
	return err
}

func (s *Supervisor) gap(r *run) {
	if r.cfg.Alive == nil || r.cfg.Signals == nil {
		return
	}
	last, ok, err := r.cfg.Alive.Load()
	if err != nil || !ok {
		return
	}
	now := s.opts.Now()
	from, to := last.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano)
	_, err = r.cfg.Signals.Append(contract.Signal{
		Kind: contract.SignalKindEvent, Collector: SupervisorCollector, TSSource: contract.TimeSourceHost,
		Attrs: contract.Attrs{"status": "gap", "tone": string(contract.ToneWarn), "from": from, "to": to},
		Event: &contract.EventBody{Body: fmt.Sprintf("collection was interrupted from %s to %s while the hub was down", from, to)},
	})
	if err != nil {
		s.opts.Log.Warn("cannot record the collection gap", "run", r.cfg.RunID, "err", err)
	}
}

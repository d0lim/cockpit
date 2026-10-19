package collect

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

type ReportStatus string

const (
	ReportOK      ReportStatus = "ok"
	ReportSkipped ReportStatus = "skipped"
	ReportError   ReportStatus = "error"
)

type CollectorReport struct {
	ID     string       `json:"id"`
	Status ReportStatus `json:"status"`
	Reason string       `json:"reason,omitempty"`
}

type ReportResult struct {
	Collectors []CollectorReport `json:"collectors"`
	Unread     []UnreadRecord    `json:"unread"`
	Status     []CollectorStatus `json:"status"`
}

type reportSlots struct {
	mu     sync.Mutex
	items  []CollectorReport
	filled []bool
	closed bool
}

func (rs *reportSlots) set(i int, rep CollectorReport) bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.closed {
		return false
	}
	rs.items[i], rs.filled[i] = rep, true
	return true
}

func (s *Supervisor) Report(ctx context.Context, runID string, env lifecycle.Env) (ReportResult, error) {
	r := s.get(runID)
	if r == nil {
		return ReportResult{}, ErrUnknownRun
	}
	_ = s.Drain(ctx, runID, lifecycle.CauseReporting)
	n := len(r.cfg.Collectors)
	slots := &reportSlots{items: make([]CollectorReport, n), filled: make([]bool, n)}
	if env != lifecycle.EnvUp {
		for i, c := range r.cfg.Collectors {
			slots.set(i, CollectorReport{ID: c.ID, Status: ReportSkipped, Reason: fmt.Sprintf("the environment is %s", env)})
		}
		return s.finish(r, slots), nil
	}
	start := s.opts.Now()
	deadline := start.Add(s.opts.ReportBudget)
	rctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	done := make(chan struct{})
	var wg sync.WaitGroup
	for i, c := range r.cfg.Collectors {
		wg.Go(func() {
			out, err := s.opts.Runner(rctx, s.spec(r, c, max(time.Until(deadline), time.Millisecond)))
			now := s.opts.Now()
			slots.mu.Lock()
			defer slots.mu.Unlock()
			if slots.closed {
				return
			}
			rep := CollectorReport{ID: c.ID, Status: ReportOK}
			if rctx.Err() != nil {
				rep.Status, rep.Reason = ReportError, fmt.Sprintf("did not finish within the report budget of %s", s.opts.ReportBudget)
				r.fold(c.ID, "collector:"+c.ID, rep.Reason, now)
			} else if reason := r.record(c.ID, out, err, now); reason != "" {
				rep.Status, rep.Reason = ReportError, reason
			}
			slots.items[i], slots.filled[i] = rep, true
		})
	}
	go func() {
		wg.Wait()
		close(done)
	}()
	timer := time.NewTimer(s.opts.ReportBudget + s.opts.DrainWait)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
	return s.finish(r, slots), nil
}

func (s *Supervisor) finish(r *run, slots *reportSlots) ReportResult {
	slots.mu.Lock()
	slots.closed = true
	now := s.opts.Now()
	for i, c := range r.cfg.Collectors {
		if !slots.filled[i] {
			reason := fmt.Sprintf("was abandoned after the report budget of %s", s.opts.ReportBudget)
			slots.items[i] = CollectorReport{ID: c.ID, Status: ReportError, Reason: reason}
			r.fold(c.ID, "collector:"+c.ID, reason, now)
		}
	}
	items := append([]CollectorReport{}, slots.items...)
	slots.mu.Unlock()
	return ReportResult{Collectors: items, Unread: s.Unread(r.cfg.RunID), Status: s.Status(r.cfg.RunID)}
}

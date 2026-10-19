package collect

import (
	"cmp"
	"fmt"
	"slices"
	"time"
)

type State string

const (
	StateArmed      State = "armed"
	StateBackoff    State = "backoff"
	StateStopped    State = "stopped"
	StateReportOnly State = "report-only"
)

type track struct {
	interval    time.Duration
	state       State
	lastSuccess time.Time
	consecutive int
	failures    int
	reason      string
	nextAt      time.Time
}

func (t *track) arm() {
	t.state = StateArmed
	if t.interval <= 0 {
		t.state = StateReportOnly
	}
	t.nextAt = time.Time{}
}

func (t *track) succeeded(now time.Time) {
	t.lastSuccess = now
	t.consecutive = 0
	t.reason = ""
	if t.state == StateBackoff {
		t.state = StateArmed
	}
}

func (t *track) failed(reason string) {
	t.failures++
	if reason == t.reason {
		t.consecutive++
	} else {
		t.consecutive = 1
	}
	t.reason = reason
}

type CollectorStatus struct {
	ID          string `json:"id"`
	State       State  `json:"state"`
	IntervalMs  int64  `json:"intervalMs"`
	LastSuccess string `json:"lastSuccess,omitempty"`
	Failures    int    `json:"failures"`
	Consecutive int    `json:"consecutive"`
	Reason      string `json:"reason,omitempty"`
	NextAt      string `json:"nextAt,omitempty"`
	Line        string `json:"line"`
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (t *track) status(id string) CollectorStatus {
	st := CollectorStatus{ID: id, State: t.state, IntervalMs: t.interval.Milliseconds(), LastSuccess: stamp(t.lastSuccess),
		Failures: t.failures, Consecutive: t.consecutive, Reason: t.reason}
	if t.state == StateArmed || t.state == StateBackoff {
		st.NextAt = stamp(t.nextAt)
	}
	switch {
	case t.state == StateBackoff:
		st.Line = fmt.Sprintf("%s: backing off after %d consecutive failures (%s); next try %s", id, t.consecutive, t.reason, st.NextAt)
	case t.state == StateStopped:
		st.Line = id + ": stopped"
	case t.failures > 0 && t.reason != "":
		st.Line = fmt.Sprintf("%s: %s, %d failures (last: %s)", id, t.state, t.failures, t.reason)
	default:
		st.Line = fmt.Sprintf("%s: %s, last success %s", id, t.state, cmp.Or(st.LastSuccess, "never"))
	}
	return st
}

func (s *Supervisor) Status(runID string) []CollectorStatus {
	r := s.get(runID)
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]CollectorStatus, 0, len(r.cfg.Collectors))
	for _, c := range r.cfg.Collectors {
		out = append(out, r.tracks[c.ID].status(c.ID))
	}
	return out
}

func (s *Supervisor) Unread(runID string) []UnreadRecord {
	r := s.get(runID)
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.ledger.snapshot())
}

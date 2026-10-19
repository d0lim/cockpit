package collect

import (
	"time"

	"github.com/d0lim/cockpit/internal/collect/worker"
)

type UnreadRecord struct {
	Collector string `json:"collector"`
	Source    string `json:"source"`
	Reason    string `json:"reason"`
	Count     int    `json:"count"`
	First     string `json:"first"`
	Last      string `json:"last"`
}

type ledger struct {
	recs  []UnreadRecord
	index map[[3]string]int
}

func (l *ledger) add(collector, source, reason string, at time.Time) {
	if l.index == nil {
		l.index = map[[3]string]int{}
	}
	key := [3]string{collector, source, reason}
	stamp := at.UTC().Format(time.RFC3339Nano)
	if i, ok := l.index[key]; ok {
		l.recs[i].Count++
		l.recs[i].Last = stamp
		return
	}
	l.index[key] = len(l.recs)
	l.recs = append(l.recs, UnreadRecord{Collector: collector, Source: source, Reason: reason, Count: 1, First: stamp, Last: stamp})
}

func (l *ledger) snapshot() []UnreadRecord {
	return append([]UnreadRecord{}, l.recs...)
}

func failure(out worker.Outcome, err error) string {
	switch {
	case err != nil:
		return "the script host could not start"
	case out.Status == worker.StatusOK:
		return ""
	case out.Status == worker.StatusKilled:
		return "killed: " + out.Kill
	case out.Kind != "":
		return out.Kind + ": " + out.Failure
	}
	return out.Failure
}

func (r *run) fold(collector, source, reason string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ledger.add(collector, source, reason, at)
}

func (r *run) record(id string, out worker.Outcome, err error, now time.Time) string {
	mask := r.cfg.Mask
	reason := mask(failure(out, err))
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range out.Unread {
		r.ledger.add(id, u.Source, mask(u.Reason), now)
	}
	t := r.tracks[id]
	if reason == "" {
		t.succeeded(now)
		return ""
	}
	r.ledger.add(id, "collector:"+id, reason, now)
	t.failed(reason)
	return reason
}

func (r *run) backoff(id string, after int, ceiling time.Duration) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.tracks[id]
	if t.consecutive < after || t.interval <= 0 {
		return 0
	}
	d := t.interval
	for range t.consecutive - after + 1 {
		d *= 2
		if d >= ceiling {
			d = ceiling
			break
		}
	}
	if t.state == StateArmed {
		t.state = StateBackoff
	}
	return d
}

func (r *run) scheduled(id string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tracks[id].nextAt = at
}

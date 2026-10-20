package screen

import (
	"errors"
	"sync"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/signal"
)

const (
	maxSignals  = 20000
	signalPage  = 5000
	signalIdle  = 10 * time.Minute
	signalReads = 20
)

type signalSnapshot struct {
	signals    []contract.Signal
	resyncs    []signal.Resync
	frozen     bool
	frozenSeq  int64
	dropped    map[string]int64
	refused    string
	collectors []api.CollectorState
}

type runSignals struct {
	mu   sync.Mutex
	seen int64
	snap signalSnapshot
	used time.Time
}

type signalCache struct {
	r    *api.Reader
	now  func() time.Time
	mu   sync.Mutex
	runs map[string]*runSignals
}

func newSignalCache(r *api.Reader) *signalCache {
	return &signalCache{r: r, now: time.Now, runs: map[string]*runSignals{}}
}

func (c *signalCache) entry(id string) *runSignals {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for k, rs := range c.runs {
		if k != id && now.Sub(rs.used) > signalIdle {
			delete(c.runs, k)
		}
	}
	rs := c.runs[id]
	if rs == nil {
		rs = &runSignals{snap: signalSnapshot{dropped: map[string]int64{}}}
		c.runs[id] = rs
	}
	rs.used = now
	return rs
}

func (c *signalCache) read(id string) signalSnapshot {
	rs := c.entry(id)
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.snap.refused != "" {
		return rs.snap
	}
	for range signalReads {
		view, err := c.r.Signals(id, rs.seen, signalPage)
		if err != nil {
			var p *api.Problem
			if !errors.As(err, &p) {
				snap := rs.snap
				snap.refused = "signal reads failed"
				return snap
			}
			rs.snap.refused = p.Message
			break
		}
		rs.snap.signals = append(rs.snap.signals, view.Signals...)
		if view.Resync != nil {
			rs.snap.resyncs = append(rs.snap.resyncs, *view.Resync)
		}
		rs.snap.frozen, rs.snap.frozenSeq, rs.snap.dropped = view.Frozen, view.FrozenSeq, view.Dropped
		rs.snap.collectors = view.Collectors
		rs.seen = max(rs.seen, view.Next)
		if len(view.Signals) < signalPage {
			break
		}
	}
	if over := len(rs.snap.signals) - maxSignals; over > 0 {
		rs.snap.signals = append([]contract.Signal(nil), rs.snap.signals[over:]...)
	}
	return rs.snap
}

func (st *state) signals() signalSnapshot { return st.s.signals.read(st.run.ID) }

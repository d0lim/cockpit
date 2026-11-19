package api

import (
	"cmp"
	"slices"
	"sync"
	"time"

	"github.com/d0lim/cockpit/internal/wire"
)

const (
	wireCacheSize = 4
	wireCacheIdle = 10 * time.Minute
)

type wireStepKey struct{ flow, step string }

type wireStepCalls struct {
	exact, traced, nested, background, timed int
	heads                                    []int
}

type wireRecord struct {
	index   wire.Index
	steps   []wireStepCalls
	outside []int
	depth   []int
	bySeq   map[int64]int
	routes  []int
	titles  map[wireStepKey]string
}

func newWireRecord(index wire.Index, titles map[wireStepKey]string) *wireRecord {
	heads := index.Heads
	r := &wireRecord{
		index: index, titles: titles, steps: make([]wireStepCalls, len(index.Steps)),
		depth: make([]int, len(heads)), bySeq: make(map[int64]int, len(heads)), routes: make([]int, len(index.Routes)),
	}
	order := make([]int, len(heads))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Or(heads[a].Started.Compare(heads[b].Started), cmp.Compare(heads[a].Seq, heads[b].Seq))
	})
	for _, i := range order {
		r.add(i)
	}
	return r
}

func (r *wireRecord) add(i int) {
	h := r.index.Heads[i]
	r.bySeq[h.Seq] = i
	for at, route := range r.index.Routes {
		if routeTakes(route.Service, route.Port, h) {
			r.routes[at]++
		}
	}
	r.depth[i] = r.depthOf(h)
	if h.Link.Kind == wire.LinkNone || h.Link.Step < 0 || h.Link.Step >= len(r.steps) {
		r.outside = append(r.outside, i)
		return
	}
	s := &r.steps[h.Link.Step]
	s.heads = append(s.heads, i)
	switch h.Link.Kind {
	case wire.LinkExact:
		s.exact++
	case wire.LinkTraced:
		s.traced++
	case wire.LinkNested:
		s.nested++
	case wire.LinkBackground:
		s.background++
	case wire.LinkTime:
		s.timed++
	}
}

func (r *wireRecord) depthOf(h wire.Head) int {
	if h.Link.Kind != wire.LinkNested && h.Link.Kind != wire.LinkTraced {
		return 0
	}
	at, ok := r.bySeq[h.Link.Parent]
	if !ok {
		if h.Link.Kind == wire.LinkNested {
			return 1
		}
		return 0
	}
	parent := r.index.Heads[at].Link
	if parent.Step != h.Link.Step || !WireLinkTied(parent.Kind) {
		return 0
	}
	return r.depth[at] + 1
}

func WireLinkTied(kind wire.LinkKind) bool {
	return kind == wire.LinkExact || kind == wire.LinkTraced || kind == wire.LinkNested
}

func routeTakes(service string, port int, h wire.Head) bool {
	return h.Target == service && h.Port == port
}

type wireKey struct {
	run, take string
	version   wire.Version
}

type wireEntry struct {
	once sync.Once
	rec  *wireRecord
	err  error
	used time.Time
}

type wireCache struct {
	now     func() time.Time
	mu      sync.Mutex
	entries map[wireKey]*wireEntry
}

func newWireCache(now func() time.Time) *wireCache {
	return &wireCache{now: now, entries: map[wireKey]*wireEntry{}}
}

func (c *wireCache) entry(key wireKey) *wireEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for k, e := range c.entries {
		replaced := k != key && k.run == key.run && k.take == key.take
		if replaced || now.Sub(e.used) > wireCacheIdle {
			delete(c.entries, k)
		}
	}
	e := c.entries[key]
	if e == nil {
		e = &wireEntry{}
		c.entries[key] = e
	}
	e.used = now
	for len(c.entries) > wireCacheSize {
		c.dropOldest(key)
	}
	return e
}

func (c *wireCache) dropOldest(keep wireKey) {
	var oldest wireKey
	var at time.Time
	for k, e := range c.entries {
		if k != keep && (at.IsZero() || e.used.Before(at)) {
			oldest, at = k, e.used
		}
	}
	delete(c.entries, oldest)
}

func (c *wireCache) read(key wireKey, load func() (*wireRecord, error)) (*wireRecord, error) {
	e := c.entry(key)
	e.once.Do(func() { e.rec, e.err = load() })
	if e.err != nil {
		c.mu.Lock()
		if c.entries[key] == e {
			delete(c.entries, key)
		}
		c.mu.Unlock()
	}
	return e.rec, e.err
}

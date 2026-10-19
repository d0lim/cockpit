package sse

import (
	"encoding/json"
	"slices"
	"sync"
	"time"
)

type Kind string

const (
	KindRuns     Kind = "runs"
	KindJob      Kind = "job"
	KindLog      Kind = "log"
	KindServices Kind = "services"
	KindSignal   Kind = "signal"
	KindResync   Kind = "resync"
	KindRefused  Kind = "signal-refused"
)

const HubTarget = "hub"

const (
	DefaultCapacity = 1000
	subscriberQueue = 256
)

var snapshotKinds = map[Kind]bool{KindRuns: true, KindServices: true}

type Event struct {
	Seq    int64
	Kind   Kind
	Target string
	At     string
	Body   any
}

func (e Event) Data() ([]byte, error) {
	fields := map[string]any{}
	if e.Body != nil {
		raw, err := json.Marshal(e.Body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, err
		}
	}
	fields["kind"], fields["target"], fields["at"] = e.Kind, e.Target, e.At
	return json.Marshal(fields)
}

type subscriber struct {
	target string
	ch     chan Event
	closed bool
}

type Bus struct {
	mu        sync.Mutex
	epoch     string
	seq       int64
	capacity  int
	now       func() time.Time
	buffers   map[string]map[Kind][]Event
	subs      map[*subscriber]struct{}
	audiences []func(target string, count int)
}

func New(epoch string, capacity int, now func() time.Time) *Bus {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	if now == nil {
		now = time.Now
	}
	return &Bus{epoch: epoch, capacity: capacity, now: now, buffers: map[string]map[Kind][]Event{}, subs: map[*subscriber]struct{}{}}
}

func (b *Bus) Epoch() string { return b.epoch }

func (b *Bus) Publish(kind Kind, target string, body any) Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	e := Event{Seq: b.seq, Kind: kind, Target: target, At: b.now().UTC().Format("2006-01-02T15:04:05.000Z"), Body: body}
	kinds := b.buffers[target]
	if kinds == nil {
		kinds = map[Kind][]Event{}
		b.buffers[target] = kinds
	}
	buf := kinds[kind]
	if snapshotKinds[kind] {
		buf = buf[:0]
	}
	buf = append(buf, e)
	if len(buf) > b.capacity {
		buf = slices.Clone(buf[len(buf)-b.capacity:])
	}
	kinds[kind] = buf
	for s := range b.subs {
		if s.target != target && target != HubTarget {
			continue
		}
		select {
		case s.ch <- e:
		default:
			b.dropLocked(s)
		}
	}
	return e
}

func (b *Bus) replayLocked(target string, after int64) []Event {
	var out []Event
	for _, t := range []string{target, HubTarget} {
		for _, buf := range b.buffers[t] {
			for _, e := range buf {
				if e.Seq > after {
					out = append(out, e)
				}
			}
		}
		if target == HubTarget {
			break
		}
	}
	slices.SortFunc(out, func(a, c Event) int { return int(a.Seq - c.Seq) })
	return out
}

func (b *Bus) Subscribe(target string, after int64) ([]Event, <-chan Event, func()) {
	b.mu.Lock()
	s := &subscriber{target: target, ch: make(chan Event, subscriberQueue)}
	b.subs[s] = struct{}{}
	replay := b.replayLocked(target, after)
	count := b.audienceLocked(target)
	listeners := slices.Clone(b.audiences)
	b.mu.Unlock()
	announce(listeners, target, count)
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			b.dropLocked(s)
			count := b.audienceLocked(target)
			listeners := slices.Clone(b.audiences)
			b.mu.Unlock()
			announce(listeners, target, count)
		})
	}
	return replay, s.ch, cancel
}

func (b *Bus) dropLocked(s *subscriber) {
	if _, ok := b.subs[s]; !ok {
		return
	}
	delete(b.subs, s)
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
}

func (b *Bus) audienceLocked(target string) int {
	n := 0
	for s := range b.subs {
		if s.target == target {
			n++
		}
	}
	return n
}

func (b *Bus) Audience(target string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.audienceLocked(target)
}

func (b *Bus) OnAudience(listener func(target string, count int)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.audiences = append(b.audiences, listener)
}

func announce(listeners []func(string, int), target string, count int) {
	for _, l := range listeners {
		l(target, count)
	}
}

func (b *Bus) Recent(target string) []Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.replayLocked(target, 0)
}

func (b *Bus) Clear(target string, kinds ...Kind) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(kinds) == 0 {
		delete(b.buffers, target)
		return
	}
	for _, k := range kinds {
		delete(b.buffers[target], k)
	}
}

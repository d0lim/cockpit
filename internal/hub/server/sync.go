package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/sse"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/signal"
)

type runsSync struct {
	sched Scheduler
	bus   *sse.Bus
	log   *slog.Logger
	wake  chan struct{}
	mu    sync.Mutex
	last  []byte
}

func newRunsSync(sched Scheduler, bus *sse.Bus, log *slog.Logger) *runsSync {
	return &runsSync{sched: sched, bus: bus, log: log, wake: make(chan struct{}, 1)}
}

func (r *runsSync) kick() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *runsSync) publish() {
	list, err := r.sched.List()
	if err != nil {
		r.log.Warn("run list failed", "error", err)
		return
	}
	body := map[string]any{"list": api.PublicList(list)}
	raw, err := json.Marshal(body)
	if err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if bytes.Equal(raw, r.last) {
		return
	}
	r.last = raw
	r.bus.Publish(sse.KindRuns, sse.HubTarget, body)
}

func (r *runsSync) loop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		r.publish()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-r.wake:
		}
	}
}

func PatternMasker(string) *mask.Masker {
	m, err := mask.New(mask.Options{Mode: mask.Hub})
	if err != nil {
		panic(err)
	}
	return m
}

type Streams struct {
	st      *store.Store
	maskFor func(string) *mask.Masker
	mu      sync.Mutex
	open    map[string]*signal.Stream
}

func NewStreams(st *store.Store, maskFor func(string) *mask.Masker) *Streams {
	if maskFor == nil {
		maskFor = PatternMasker
	}
	return &Streams{st: st, maskFor: maskFor, open: map[string]*signal.Stream{}}
}

func (s *Streams) Get(runID string) (*signal.Stream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stream, ok := s.open[runID]; ok {
		return stream, nil
	}
	stream, err := signal.OpenRun(s.st, runID, signal.Options{Masker: func() *mask.Masker { return s.maskFor(runID) }})
	if err != nil {
		return nil, err
	}
	s.open[runID] = stream
	return stream, nil
}

func (s *Streams) Close(runID string) error {
	s.mu.Lock()
	stream, ok := s.open[runID]
	delete(s.open, runID)
	s.mu.Unlock()
	if !ok {
		return nil
	}
	return stream.Close()
}

func (s *Streams) Read(runID string, req signal.ReadRequest) (signal.ReadResponse, error) {
	stream, err := s.Get(runID)
	if err != nil {
		return signal.ReadResponse{}, err
	}
	return stream.Read(req)
}

func (s *Streams) CloseAll() {
	s.mu.Lock()
	open := s.open
	s.open = map[string]*signal.Stream{}
	s.mu.Unlock()
	for _, stream := range open {
		stream.Close()
	}
}

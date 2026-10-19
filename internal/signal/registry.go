package signal

import (
	"errors"
	"sync"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
)

type Registry struct {
	st      *store.Store
	maskFor func(string) *mask.Masker
	mu      sync.Mutex
	open    map[string]*Stream
}

func NewRegistry(st *store.Store, maskFor func(runID string) *mask.Masker) *Registry {
	return &Registry{st: st, maskFor: maskFor, open: map[string]*Stream{}}
}

func (r *Registry) Get(runID string) (*Stream, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.open[runID]; ok {
		return s, nil
	}
	if m, err := r.st.ReadMeta(runID); err == nil && m != nil && m.Env == lifecycle.EnvDestroyed {
		return nil, ErrRunDestroyed
	}
	s, err := OpenRun(r.st, runID, Options{Masker: func() *mask.Masker { return r.maskFor(runID) }})
	if err != nil {
		return nil, err
	}
	r.open[runID] = s
	return s, nil
}

func (r *Registry) Close(runID string) error {
	r.mu.Lock()
	s, ok := r.open[runID]
	delete(r.open, runID)
	r.mu.Unlock()
	if !ok {
		return nil
	}
	return s.Close()
}

func (r *Registry) Remove(runID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var err error
	if s, ok := r.open[runID]; ok {
		delete(r.open, runID)
		err = s.Close()
	}
	return errors.Join(err, RemoveRun(r.st, runID))
}

func (r *Registry) Read(runID string, req ReadRequest) (ReadResponse, error) {
	s, err := r.Get(runID)
	if err != nil {
		return ReadResponse{}, err
	}
	return s.Read(req)
}

func (r *Registry) CloseAll() {
	r.mu.Lock()
	open := r.open
	r.open = map[string]*Stream{}
	r.mu.Unlock()
	for _, s := range open {
		s.Close()
	}
}

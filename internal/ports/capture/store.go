package capture

import (
	"maps"
	"slices"
	"sync"

	"github.com/d0lim/cockpit/internal/ports"
)

type Store struct {
	mu      sync.Mutex
	byName  map[string]string
	persist map[string]bool
	mask    ports.Mask
}

func NewStore(persisted map[string]string) *Store {
	s := &Store{byName: map[string]string{}, persist: map[string]bool{}, mask: ports.Identity}
	for name, value := range persisted {
		s.Set(name, value, true)
	}
	return s
}

func (s *Store) Set(name, value string, persist bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byName[name] = value
	if persist {
		s.persist[name] = true
		return
	}
	delete(s.persist, name)
}

func (s *Store) Get(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.byName[name]
	if !ok {
		return "", ports.BadArgs(s.mask, "capture "+name+" is missing — no earlier step or prerequisite flow left it")
	}
	return value, nil
}

func (s *Store) Values() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := slices.Collect(maps.Values(s.byName))
	slices.Sort(values)
	return values
}

func (s *Store) Persisted() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for name := range s.persist {
		out[name] = s.byName[name]
	}
	return out
}

package store

import (
	"fmt"
	"slices"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

func (s *Store) MustRead(id string) (Meta, error) {
	m, err := s.ReadMeta(id)
	if err != nil {
		return Meta{}, err
	}
	if m == nil {
		return Meta{}, fmt.Errorf("no run record: %s", id)
	}
	return *m, nil
}

func (s *Store) PatchMeta(id string, change func(*Meta)) (Meta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.patchMeta(id, change)
}

func (s *Store) patchMeta(id string, change func(*Meta)) (Meta, error) {
	m, err := s.MustRead(id)
	if err != nil {
		return Meta{}, err
	}
	if change != nil {
		change(&m)
	}
	if err := s.writeMeta(m); err != nil {
		return Meta{}, err
	}
	return m, nil
}

func (s *Store) ApplyEvent(id string, event lifecycle.Event, d lifecycle.Detail, extra func(*Meta)) (Meta, error) {
	before, after, err := s.applyEvent(id, event, d, extra)
	if err != nil {
		return after, err
	}
	notify(s.home, Transition{Before: before, Event: event, Detail: d, After: after})
	return after, nil
}

func (s *Store) applyEvent(id string, event lifecycle.Event, d lifecycle.Detail, extra func(*Meta)) (lifecycle.State, Meta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.MustRead(id)
	if err != nil {
		return lifecycle.State{}, Meta{}, err
	}
	next, err := lifecycle.Transition(&m.State, event, d)
	if err != nil {
		return m.State, m, err
	}
	after, err := s.patchMeta(id, func(m *Meta) {
		if extra != nil {
			extra(m)
		}
		m.State = next
	})
	return m.State, after, err
}

func HoldsLease(m Meta) bool {
	if m.Slot == nil || m.Mark != lifecycle.MarkNone || m.Phase == lifecycle.Queued {
		return false
	}
	return m.Phase != lifecycle.Done || m.Env == lifecycle.EnvUp || m.Env == lifecycle.EnvStopped
}

func queuedRuns(runs []Meta) []Meta {
	var out []Meta
	for _, m := range runs {
		if m.Phase == lifecycle.Queued && m.Mark == lifecycle.MarkNone {
			out = append(out, m)
		}
	}
	slices.SortFunc(out, ByCreated)
	return out
}

func RebuildState(runs []Meta) HubState {
	st := EmptyState()
	for _, m := range queuedRuns(runs) {
		st.Queue = append(st.Queue, m.ID)
	}
	for _, m := range runs {
		if HoldsLease(m) {
			st.Leases[m.ID] = *m.Slot
		}
	}
	return st
}

func NormalizedState(runs []Meta, st HubState, broken string) HubState {
	if broken != "" {
		return RebuildState(runs)
	}
	byID := make(map[string]Meta, len(runs))
	for _, m := range runs {
		byID[m.ID] = m
	}
	out := EmptyState()
	seen := map[string]bool{}
	for _, id := range st.Queue {
		if m, ok := byID[id]; ok && m.Phase == lifecycle.Queued && !seen[id] {
			out.Queue = append(out.Queue, id)
			seen[id] = true
		}
	}
	for _, m := range queuedRuns(runs) {
		if !seen[m.ID] {
			out.Queue = append(out.Queue, m.ID)
		}
	}
	for id, slot := range st.Leases {
		if m, ok := byID[id]; ok && HoldsLease(m) {
			out.Leases[id] = slot
		}
	}
	for _, m := range runs {
		if _, ok := out.Leases[m.ID]; HoldsLease(m) && !ok {
			out.Leases[m.ID] = *m.Slot
		}
	}
	return out
}

func (s *Store) CurrentState() (HubState, error) {
	read := s.ReadState()
	if read.Broken == "" {
		return read.State, nil
	}
	list, err := s.ListRuns()
	if err != nil {
		return HubState{}, err
	}
	return RebuildState(list.Runs), nil
}

func (s *Store) UpdateState(fn func(HubState) HubState) (HubState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, err := s.CurrentState()
	if err != nil {
		return HubState{}, err
	}
	next := fn(cur)
	if err := s.writeState(next); err != nil {
		return HubState{}, err
	}
	return next, nil
}

func (st HubState) Without(id string) HubState {
	out := HubState{SchemaVersion: st.SchemaVersion, Queue: []string{}, Leases: Leases{}}
	for _, q := range st.Queue {
		if q != id {
			out.Queue = append(out.Queue, q)
		}
	}
	for k, v := range st.Leases {
		out.Leases[k] = v
	}
	return out
}

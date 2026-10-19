package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

const (
	stateFile = "state.json"
	metaFile  = "meta.json"
	runsDir   = "runs"
)

var (
	registryMu sync.Mutex
	registry   = map[string]*sync.Mutex{}
)

type Store struct {
	home string
	mu   *sync.Mutex
}

func Open(home string) *Store {
	abs, err := filepath.Abs(home)
	if err != nil {
		abs = filepath.Clean(home)
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	mu, ok := registry[abs]
	if !ok {
		mu = &sync.Mutex{}
		registry[abs] = mu
	}
	return &Store{home: abs, mu: mu}
}

func (s *Store) Home() string { return s.home }

func (s *Store) RunDir(id string) (string, error) {
	if !IsRunID(id) {
		return "", fmt.Errorf("not a run id: %q", id)
	}
	return filepath.Join(s.home, runsDir, id), nil
}

type StateRead struct {
	State  HubState
	Broken string
}

func (s *Store) ReadState() StateRead {
	data, err := os.ReadFile(filepath.Join(s.home, stateFile))
	if errors.Is(err, fs.ErrNotExist) {
		return StateRead{State: EmptyState()}
	}
	if err != nil {
		return StateRead{State: EmptyState(), Broken: stateFile + " cannot be read"}
	}
	var st HubState
	if json.Unmarshal(data, &st) != nil {
		return StateRead{State: EmptyState(), Broken: stateFile + " is not JSON"}
	}
	if !validState(st, data) {
		return StateRead{State: EmptyState(), Broken: fmt.Sprintf("%s does not match schema version %d", stateFile, StateSchemaVersion)}
	}
	return StateRead{State: st}
}

func validState(st HubState, raw []byte) bool {
	var shape map[string]json.RawMessage
	if json.Unmarshal(raw, &shape) != nil || shape["queue"] == nil || shape["leases"] == nil {
		return false
	}
	if st.SchemaVersion != StateSchemaVersion || st.Queue == nil || st.Leases == nil {
		return false
	}
	for _, id := range st.Queue {
		if !IsRunID(id) {
			return false
		}
	}
	for id, slot := range st.Leases {
		if !IsRunID(id) || slot < 0 {
			return false
		}
	}
	return true
}

func (s *Store) WriteState(st HubState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeState(st)
}

func (s *Store) writeState(st HubState) error {
	if st.Queue == nil {
		st.Queue = []string{}
	}
	if st.Leases == nil {
		st.Leases = Leases{}
	}
	if err := ensurePrivateDir(s.home); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return WriteAtomic(filepath.Join(s.home, stateFile), append(data, '\n'))
}

func (s *Store) ReadMeta(id string) (*Meta, error) {
	dir, err := s.RunDir(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, metaFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("cannot read %s: %s", metaFile, id)
	}
	return &m, nil
}

func (s *Store) WriteMeta(m Meta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeMeta(m)
}

func (s *Store) writeMeta(m Meta) error {
	dir, err := s.RunDir(m.ID)
	if err != nil {
		return err
	}
	if leak := credentialPathAt(m); leak != "" {
		return fmt.Errorf("meta does not record credential file paths: %s", leak)
	}
	for _, d := range []string{s.home, filepath.Join(s.home, runsDir), dir} {
		if err := ensurePrivateDir(d); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return WriteAtomic(filepath.Join(dir, metaFile), append(data, '\n'))
}

type RunList struct {
	Runs        []Meta
	State       HubState
	StateBroken string
}

func (s *Store) ListRuns() (RunList, error) {
	read := s.ReadState()
	entries, err := os.ReadDir(filepath.Join(s.home, runsDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return RunList{}, err
	}
	runs := []Meta{}
	for _, e := range entries {
		if !IsRunID(e.Name()) {
			continue
		}
		m, err := s.ReadMeta(e.Name())
		if err != nil {
			return RunList{}, err
		}
		if m != nil {
			runs = append(runs, *m)
		}
	}
	slices.SortFunc(runs, ByCreated)
	return RunList{Runs: runs, State: read.State, StateBroken: read.Broken}, nil
}

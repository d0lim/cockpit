package signal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
)

const (
	kvDir    = "kv"
	KVMaxLen = 1 << 20
)

func (s *Stream) kvName(collector string) (string, error) {
	if !ValidCollector(collector) {
		return "", fmt.Errorf("collector id %q does not match %s", collector, collectorName)
	}
	return kvDir + "/" + collector + ".json", nil
}

func (s *Stream) KVGet(collector string) (map[string]any, error) {
	name, err := s.kvName(collector)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.root.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("cannot read the state of collector %s: %w", collector, err)
	}
	return out, nil
}

func (s *Stream) KVPut(collector string, value map[string]any) error {
	name, err := s.kvName(collector)
	if err != nil {
		return err
	}
	if value == nil {
		value = map[string]any{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > KVMaxLen {
		return fmt.Errorf("the state of collector %s is %d bytes, over the limit of %d", collector, len(data), KVMaxLen)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.root.MkdirAll(kvDir, 0o700); err != nil {
		return err
	}
	return writeRootFile(s.root, name, data)
}

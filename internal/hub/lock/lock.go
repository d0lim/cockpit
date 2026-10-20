package lock

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	fileName = "hub.lock"
	attempts = 3
)

type Lock struct {
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	StartedAt string `json:"startedAt"`
	Path      string `json:"-"`
}

var (
	registryMu sync.Mutex
	registry   = map[string]*Lock{}
)

func lockPath(home string) string {
	abs, err := filepath.Abs(home)
	if err != nil {
		abs = filepath.Clean(home)
	}
	return filepath.Join(abs, fileName)
}

func Acquire(home string, port int) (*Lock, error) {
	path := lockPath(home)
	registryMu.Lock()
	defer registryMu.Unlock()
	if held, ok := registry[path]; ok {
		return held, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	for range attempts {
		mine := &Lock{PID: os.Getpid(), Port: port, StartedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), Path: path}
		created, err := createExclusive(mine)
		if err != nil {
			return nil, err
		}
		if created {
			registry[path] = mine
			return mine, nil
		}
		raw, err := readIfPresent(path)
		if err != nil {
			return nil, err
		}
		if raw == nil {
			continue
		}
		owner, err := parse(raw, path)
		if err != nil {
			return nil, err
		}
		if owner.PID == os.Getpid() {
			registry[path] = owner
			return owner, nil
		}
		if alive(owner.PID) {
			return nil, fmt.Errorf("a hub is already running: http://127.0.0.1:%d (pid %d, %s)", owner.Port, owner.PID, path)
		}
		if again, _ := readIfPresent(path); bytes.Equal(again, raw) {
			os.Remove(path)
		}
	}
	return nil, fmt.Errorf("could not take hub.lock: another hub is starting at the same time (%s)", path)
}

func Release(home string) {
	path := lockPath(home)
	registryMu.Lock()
	delete(registry, path)
	registryMu.Unlock()
	raw, err := readIfPresent(path)
	if err != nil || raw == nil {
		return
	}
	var body Lock
	if json.Unmarshal(raw, &body) == nil && body.PID == os.Getpid() {
		os.Remove(path)
	}
}

func Holder(home string) (*Lock, bool) {
	path := lockPath(home)
	raw, err := readIfPresent(path)
	if err != nil || raw == nil {
		return nil, false
	}
	owner, err := parse(raw, path)
	if err != nil || !alive(owner.PID) {
		return nil, false
	}
	return owner, true
}

func createExclusive(l *Lock) (bool, error) {
	f, err := os.OpenFile(l.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	data, _ := json.Marshal(l)
	_, err = f.Write(append(data, '\n'))
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err == nil, err
}

func parse(raw []byte, path string) (*Lock, error) {
	var fields map[string]json.RawMessage
	var body Lock
	if json.Unmarshal(raw, &fields) == nil && json.Unmarshal(raw, &body) == nil &&
		fields["pid"] != nil && fields["port"] != nil && fields["startedAt"] != nil && body.PID > 0 {
		body.Path = path
		return &body, nil
	}
	return nil, fmt.Errorf("cannot read hub.lock: if no hub is running, delete %s and start again", path)
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func readIfPresent(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

package collect

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"time"
)

const aliveFile = "collect-alive.json"

type Alive interface {
	Load() (time.Time, bool, error)
	Save(at time.Time) error
	Clear() error
}

type FileAlive struct {
	Dir string
}

type aliveRecord struct {
	At string `json:"at"`
}

func (f FileAlive) root() (*os.Root, error) {
	return os.OpenRoot(f.Dir)
}

func (f FileAlive) Load() (time.Time, bool, error) {
	root, err := f.root()
	if err != nil {
		return time.Time{}, false, err
	}
	defer root.Close()
	raw, err := root.ReadFile(aliveFile)
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	var rec aliveRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return time.Time{}, false, err
	}
	at, err := time.Parse(time.RFC3339Nano, rec.At)
	if err != nil {
		return time.Time{}, false, err
	}
	return at, true, nil
}

func (f FileAlive) Save(at time.Time) error {
	root, err := f.root()
	if err != nil {
		return err
	}
	defer root.Close()
	raw, err := json.Marshal(aliveRecord{At: at.UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	tmp := aliveFile + ".tmp"
	if err := root.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return root.Rename(tmp, aliveFile)
}

func (f FileAlive) Clear() error {
	root, err := f.root()
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(aliveFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

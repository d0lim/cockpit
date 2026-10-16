package capture

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sync"

	"github.com/d0lim/cockpit/internal/contract"
)

const fileName = "context.json"

type Context struct {
	mu       sync.Mutex
	dir      string
	Captures *Store
	steps    map[string]int64
}

func Load(dir string) (*Context, error) {
	c := &Context{dir: dir, steps: map[string]int64{}}
	raw, err := os.ReadFile(filepath.Join(dir, fileName))
	if errors.Is(err, fs.ErrNotExist) {
		c.Captures = NewStore(nil)
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	parsed, err := contract.ReadRunnerFile[contract.Context](bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	c.Captures = NewStore(parsed.Captures)
	if parsed.TOTPSteps != nil {
		c.steps = maps.Clone(parsed.TOTPSteps)
	}
	return c, nil
}

func (c *Context) LastStep(loginID string) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	step, ok := c.steps[loginID]
	return step, ok
}

func (c *Context) CommitStep(loginID string, step int64) error {
	c.mu.Lock()
	c.steps[loginID] = step
	c.mu.Unlock()
	return c.Save()
}

func (c *Context) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	doc := contract.Context{
		SchemaVersion: contract.RunnerSchemaVersion,
		Captures:      c.Captures.Persisted(),
		TOTPSteps:     maps.Clone(c.steps),
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(c.dir, fileName), append(data, '\n'))
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".context-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

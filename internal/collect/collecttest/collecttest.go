package collecttest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/ports"
)

const childMarker = "--"

func MaybeServeChild() {
	i := slices.Index(os.Args, childMarker)
	if i >= 0 && i+1 < len(os.Args) && os.Args[i+1] == worker.SubCommand {
		os.Exit(worker.Main(context.Background(), os.Stdin, os.Stdout, os.Stderr))
	}
}

func Command() []string {
	return []string{os.Args[0], "-test.run=^$", childMarker, worker.SubCommand}
}

func Bundle(t testing.TB, collectorID, body string) string {
	t.Helper()
	dir := t.TempDir()
	src := "import { collector } from \"cockpit:collect\";\nexport default collector({ id: \"" + collectorID + "\", run: async (ctx) => {\n" + body + "\n} });\n"
	if err := os.WriteFile(filepath.Join(dir, "collector.ts"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	code, err := worker.Bundle(context.Background(), dir, "collector.ts")
	if err != nil {
		t.Fatal(err)
	}
	return code
}

type Call struct {
	Method string
	Args   []json.RawMessage
}

type Recorder struct {
	mu     sync.Mutex
	calls  []Call
	Result any
	Err    error
	Delay  time.Duration
}

func (r *Recorder) Call(ctx context.Context, method string, args []json.RawMessage) (any, error) {
	r.mu.Lock()
	r.calls = append(r.calls, Call{Method: method, Args: args})
	r.mu.Unlock()
	if r.Delay > 0 {
		select {
		case <-time.After(r.Delay):
		case <-ctx.Done():
		}
	}
	return r.Result, r.Err
}

func (r *Recorder) Calls() []Call {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

func MaskOf(values ...string) ports.Mask {
	return func(s string) string {
		for _, v := range values {
			s = strings.ReplaceAll(s, v, "****")
		}
		return s
	}
}

type Frames struct {
	mu    sync.Mutex
	lines map[string][]string
}

func (f *Frames) Tap(dir string, line []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lines == nil {
		f.lines = map[string][]string{}
	}
	f.lines[dir] = append(f.lines[dir], string(line))
}

func (f *Frames) Lines(dir string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.lines[dir])
}

func (f *Frames) Text(dir string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.lines[dir], "")
}

func Spec(code string, allowed []string, handlers map[string]ports.Handler) worker.Spec {
	return worker.Spec{
		Command:   Command(),
		Collector: "toy-db",
		Code:      code,
		Allowed:   allowed,
		Ports:     handlers,
		Limits:    worker.Limits{Deadline: 5 * time.Second},
	}
}

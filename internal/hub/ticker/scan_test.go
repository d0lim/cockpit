package ticker

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/reconcile"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type scanDocker struct {
	calls    atomic.Int32
	onSecond func()
}

func (s *scanDocker) ComposeLs(context.Context) ([]reconcile.Project, error) {
	if s.calls.Add(1) < 2 {
		return []reconcile.Project{}, nil
	}
	if s.onSecond != nil {
		s.onSecond()
	}
	return nil, errors.New("docker compose: signal: killed")
}

func (s *scanDocker) Info(context.Context) bool { return true }

func (s *scanDocker) Labelled(context.Context, string) (reconcile.Labelled, error) {
	return reconcile.Labelled{}, nil
}

func scanLog(t *testing.T, cancelOnScan bool) string {
	t.Helper()
	var buf lockedBuffer
	docker := &scanDocker{}
	c, _ := core(t, docker, &quietEnv{}, func(d *Deps) { d.Log = slog.New(slog.NewTextHandler(&buf, nil)) })
	if cancelOnScan {
		docker.onSecond = c.cancelBase
	}
	c.tickOnce()
	c.tickOnce()
	if docker.calls.Load() < 2 {
		t.Fatalf("the project scan ran %d times", docker.calls.Load())
	}
	return buf.String()
}

func TestAProjectScanCancelledByShutdownIsNotLoggedAndARealFailureIs(t *testing.T) {
	if got := scanLog(t, true); strings.Contains(got, "project scan failed") {
		t.Fatalf("a cancelled scan was logged as a failure:\n%s", got)
	}
	if got := scanLog(t, false); !strings.Contains(got, "project scan failed") {
		t.Fatalf("canary: a real scan failure was not logged:\n%s", got)
	}
}

package collecttest

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"
)

const clockWatchdog = 30 * time.Second

type sleeper struct {
	at   time.Time
	wake chan struct{}
}

type Clock struct {
	mu       sync.Mutex
	now      time.Time
	sleepers []*sleeper
	changed  chan struct{}
}

func NewClock(start time.Time) *Clock {
	return &Clock{now: start, changed: make(chan struct{})}
}

func (c *Clock) notify() {
	close(c.changed)
	c.changed = make(chan struct{})
}

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *Clock) Sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	c.mu.Lock()
	s := &sleeper{at: c.now.Add(d), wake: make(chan struct{})}
	c.sleepers = append(c.sleepers, s)
	c.notify()
	c.mu.Unlock()
	select {
	case <-s.wake:
		return ctx.Err() == nil
	case <-ctx.Done():
		c.mu.Lock()
		c.sleepers = slices.DeleteFunc(c.sleepers, func(o *sleeper) bool { return o == s })
		c.notify()
		c.mu.Unlock()
		return false
	}
}

func (c *Clock) Sleepers() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sleepers)
}

func (c *Clock) AwaitSleepers(t testing.TB, n int) {
	t.Helper()
	watchdog := time.NewTimer(clockWatchdog)
	defer watchdog.Stop()
	for {
		c.mu.Lock()
		got, changed := len(c.sleepers), c.changed
		c.mu.Unlock()
		if got == n {
			return
		}
		select {
		case <-changed:
		case <-watchdog.C:
			t.Fatalf("%d goroutines sleep on the clock, want %d", got, n)
		}
	}
}

func (c *Clock) Due() (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.due()
}

func (c *Clock) due() (time.Time, bool) {
	if len(c.sleepers) == 0 {
		return time.Time{}, false
	}
	at := c.sleepers[0].at
	for _, s := range c.sleepers[1:] {
		if s.at.Before(at) {
			at = s.at
		}
	}
	return at, true
}

func (c *Clock) Next() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	at, ok := c.due()
	if !ok {
		return false
	}
	c.now = at
	c.sleepers = slices.DeleteFunc(c.sleepers, func(s *sleeper) bool {
		if s.at.After(at) {
			return false
		}
		close(s.wake)
		return true
	})
	c.notify()
	return true
}

func (c *Clock) RunUntil(t testing.TB, end time.Time, sleepers int) int {
	t.Helper()
	c.AwaitSleepers(t, sleepers)
	steps := 0
	for {
		at, ok := c.Due()
		if !ok || at.After(end) {
			return steps
		}
		c.Next()
		steps++
		c.AwaitSleepers(t, sleepers)
	}
}

package collecttest

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestClockWakesSleepersInDueOrderAndRunUntilStopsBeforeTheEnd(t *testing.T) {
	start := time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC)
	c := NewClock(start)
	var mu sync.Mutex
	var woke []time.Duration
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for _, every := range []time.Duration{30 * time.Millisecond, 50 * time.Millisecond} {
		wg.Go(func() {
			for c.Sleep(ctx, every) {
				mu.Lock()
				woke = append(woke, c.Now().Sub(start))
				mu.Unlock()
			}
		})
	}
	steps := c.RunUntil(t, start.Add(100*time.Millisecond), 2)
	cancel()
	wg.Wait()
	want := []time.Duration{30, 50, 60, 90, 100}
	mu.Lock()
	defer mu.Unlock()
	if steps != 5 || len(woke) != len(want) || c.Sleepers() != 0 {
		t.Fatalf("steps %d woke %v sleepers %d", steps, woke, c.Sleepers())
	}
	for i, ms := range want {
		if woke[i] != ms*time.Millisecond {
			t.Fatalf("wake %d at %v, want %vms (all %v)", i, woke[i], ms, woke)
		}
	}
	if c.Sleep(context.Background(), 0) != true || c.Next() {
		t.Fatal("a zero sleep blocks, or an empty clock advances")
	}
}

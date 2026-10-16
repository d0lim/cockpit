package clock

import (
	"context"
	"errors"
	"testing"
	"time"
)

func Test밀리초_시계와_취소되는_sleep(t *testing.T) {
	fixed := time.UnixMilli(1_700_000_000_123)
	c := Clock{Now: func() time.Time { return fixed }}
	if c.Millis() != 1_700_000_000_123 {
		t.Fatalf("%d", c.Millis())
	}
	start := time.Now()
	if err := System().Sleep(context.Background(), 20); err != nil || time.Since(start) < 20*time.Millisecond {
		t.Fatalf("sleep: %v after %v", err, time.Since(start))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := System().Sleep(ctx, 10_000); !errors.Is(err, context.Canceled) {
		t.Fatalf("취소: %v", err)
	}
	if err := System().Sleep(context.Background(), -5); err != nil {
		t.Fatalf("음수: %v", err)
	}
}

package clock

import (
	"context"
	"time"
)

type Clock struct {
	Now func() time.Time
}

func System() Clock {
	return Clock{Now: time.Now}
}

func (c Clock) Millis() int64 {
	return c.Now().UnixMilli()
}

func (c Clock) Sleep(ctx context.Context, ms int64) error {
	if ms <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(time.Duration(ms) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

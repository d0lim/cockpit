package worker

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	DefaultDeadline    = 30 * time.Second
	DefaultMaxCalls    = 10_000
	DefaultMaxInFlight = 16
	DefaultMaxRSS      = 512 << 20
	DefaultMaxFrame    = 1 << 20
	DefaultMaxStderr   = 64 << 10
	DefaultRSSPoll     = 100 * time.Millisecond
	killGrace          = 2 * time.Second
)

const (
	KillDeadline = "deadline"
	KillCalls    = "call budget"
	KillMemory   = "memory"
	KillFrame    = "frame over the size limit"
	KillProtocol = "protocol"
	KillCanceled = "canceled"
)

type Limits struct {
	Deadline    time.Duration
	MaxCalls    int
	MaxInFlight int
	MaxRSS      int64
	MaxFrame    int
	MaxStderr   int
	RSSPoll     time.Duration
}

func DefaultLimits() Limits {
	return Limits{
		Deadline: DefaultDeadline, MaxCalls: DefaultMaxCalls, MaxInFlight: DefaultMaxInFlight,
		MaxRSS: DefaultMaxRSS, MaxFrame: DefaultMaxFrame, MaxStderr: DefaultMaxStderr, RSSPoll: DefaultRSSPoll,
	}
}

func (l Limits) filled() Limits {
	d := DefaultLimits()
	if l.Deadline <= 0 {
		l.Deadline = d.Deadline
	}
	if l.MaxCalls <= 0 {
		l.MaxCalls = d.MaxCalls
	}
	if l.MaxInFlight <= 0 {
		l.MaxInFlight = d.MaxInFlight
	}
	if l.MaxRSS <= 0 {
		l.MaxRSS = d.MaxRSS
	}
	if l.MaxFrame <= 0 {
		l.MaxFrame = d.MaxFrame
	}
	if l.MaxStderr <= 0 {
		l.MaxStderr = d.MaxStderr
	}
	if l.RSSPoll <= 0 {
		l.RSSPoll = d.RSSPoll
	}
	return l
}

func ChildEnv(getenv func(string) string) []string {
	return []string{"PATH=" + getenv("PATH")}
}

func RSS(ctx context.Context, pid int) (int64, bool) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, false
	}
	kb, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, false
	}
	return kb * 1024, true
}

func killGroup(pid int) {
	if pid <= 1 || pid == os.Getpid() {
		return
	}
	syscall.Kill(-pid, syscall.SIGKILL)
}

func (h *host) watchRSS(ctx context.Context, pid int) {
	t := time.NewTicker(h.limits.RSSPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if rss, ok := h.rss(ctx, pid); ok && rss > h.limits.MaxRSS {
				h.kill(KillMemory)
				return
			}
		}
	}
}

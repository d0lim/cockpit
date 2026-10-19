package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports"
)

type Status string

const (
	StatusOK     Status = "ok"
	StatusFailed Status = "failed"
	StatusKilled Status = "killed"
)

const (
	TapToChild   = "to-child"
	TapFromChild = "from-child"
)

type UnreadRecord struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

type Outcome struct {
	Status          Status
	Kill            string
	Kind            string
	Failure         string
	Calls           int
	Stderr          string
	StderrTruncated bool
	Unread          []UnreadRecord
}

type Spec struct {
	Command   []string
	Collector string
	Code      string
	Allowed   []string
	Ports     map[string]ports.Handler
	Mask      ports.Mask
	Limits    Limits
	Tap       func(dir string, line []byte)
	RSS       func(ctx context.Context, pid int) (int64, bool)
	Getenv    func(string) string
	Log       *slog.Logger
}

type host struct {
	spec     Spec
	limits   Limits
	rss      func(ctx context.Context, pid int) (int64, bool)
	out      *writer
	pid      int
	mu       sync.Mutex
	reason   string
	calls    int
	inflight int
	unread   []UnreadRecord
	serving  sync.WaitGroup
}

func SelfCommand() ([]string, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return []string{self, SubCommand}, nil
}

func Run(ctx context.Context, spec Spec) (Outcome, error) {
	if len(spec.Command) == 0 {
		return Outcome{}, errors.New("no script host command")
	}
	if spec.Mask == nil {
		spec.Mask = ports.Identity
	}
	if spec.Getenv == nil {
		spec.Getenv = os.Getenv
	}
	if spec.Log == nil {
		spec.Log = slog.New(slog.DiscardHandler)
	}
	h := &host{spec: spec, limits: spec.Limits.filled(), rss: spec.RSS}
	if h.rss == nil {
		h.rss = RSS
	}
	cmd := exec.Command(spec.Command[0], spec.Command[1:]...)
	cmd.Env = ChildEnv(spec.Getenv)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = killGrace
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Outcome{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Outcome{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Outcome{}, err
	}
	if err := cmd.Start(); err != nil {
		return Outcome{}, fmt.Errorf("cannot start the script host: %w", err)
	}
	h.pid = cmd.Process.Pid
	h.out = &writer{w: tapped{stdin, h.tap(TapToChild)}}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	timer := time.AfterFunc(h.limits.Deadline+killGrace, func() { h.kill(KillDeadline) })
	defer timer.Stop()
	stop := context.AfterFunc(ctx, func() { h.kill(KillCanceled) })
	defer stop()
	go h.watchRSS(runCtx, h.pid)
	var errText capped
	errDone := make(chan struct{})
	go func() {
		errText.copy(stderr, h.limits.MaxStderr)
		close(errDone)
	}()
	setup, _ := json.Marshal(Setup{Collector: spec.Collector, Code: spec.Code, Allowed: spec.Allowed, DeadlineMs: h.limits.Deadline.Milliseconds()})
	if err := h.out.raw(setup); err != nil {
		h.kill(KillProtocol)
	}
	done := h.read(runCtx, bufio.NewReaderSize(stdout, 1<<16))
	h.serving.Wait()
	stdin.Close()
	waitErr := cmd.Wait()
	<-errDone
	cancel()
	return h.outcome(done, waitErr, &errText), nil
}

func (h *host) tap(dir string) func([]byte) {
	return func(line []byte) {
		if h.spec.Tap != nil {
			h.spec.Tap(dir, line)
		}
	}
}

func (h *host) kill(reason string) {
	h.mu.Lock()
	if h.reason == "" {
		h.reason = reason
	}
	h.mu.Unlock()
	killGroup(h.pid)
}

func (h *host) killed() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.reason
}

func (h *host) read(ctx context.Context, r *bufio.Reader) *Done {
	var done *Done
	for {
		line, err := readLine(r, h.limits.MaxFrame)
		if err != nil && !errors.Is(err, io.EOF) {
			h.kill(KillFrame)
			_, _ = io.Copy(io.Discard, r)
			return done
		}
		if len(line) > 0 {
			h.tap(TapFromChild)(line)
			var f Frame
			if json.Unmarshal(line, &f) != nil {
				h.kill(KillProtocol)
			} else if f.Done != nil {
				done = f.Done
			} else if f.ID > 0 {
				h.dispatch(ctx, f)
			}
		}
		if err != nil {
			return done
		}
	}
}

func (h *host) dispatch(ctx context.Context, f Frame) {
	h.mu.Lock()
	h.calls++
	over := h.calls > h.limits.MaxCalls
	busy := h.inflight >= h.limits.MaxInFlight
	if !over && !busy {
		h.inflight++
	}
	h.mu.Unlock()
	switch {
	case over:
		h.kill(KillCalls)
	case busy:
		h.reply(f.ID, f.Port, nil, ports.Limit(h.spec.Mask, fmt.Sprintf("more than %d port calls are in flight", h.limits.MaxInFlight)))
	default:
		h.serving.Add(1)
		go func() {
			defer h.serving.Done()
			result, err := h.serve(ctx, f)
			h.mu.Lock()
			h.inflight--
			h.mu.Unlock()
			h.reply(f.ID, f.Port, result, err)
		}()
	}
}

func (h *host) serve(ctx context.Context, f Frame) (any, error) {
	if !slices.Contains(h.spec.Allowed, f.Port) {
		return nil, ports.Denied(h.spec.Mask, fmt.Sprintf("port %q is not allowed for collector %s", f.Port, h.spec.Collector))
	}
	handler := h.spec.Ports[f.Port]
	if handler == nil {
		return nil, ports.Unavailable(h.spec.Mask, fmt.Sprintf("port %q is not available on this hub", f.Port))
	}
	return handler.Call(ctx, f.Method, f.Args)
}

func (h *host) reply(id int64, port string, result any, err error) {
	f := Frame{ID: id}
	if err != nil {
		known := &engine.PortError{Code: engine.PortUnavailable, Message: "port call failed"}
		errors.As(err, &known)
		f.Error = &FrameError{Code: known.Code, Message: h.spec.Mask(known.Message)}
		if known.Code == engine.PortDenied {
			h.mu.Lock()
			h.unread = append(h.unread, UnreadRecord{Source: "port:" + port, Reason: f.Error.Message})
			h.mu.Unlock()
		}
	} else {
		raw, merr := json.Marshal(result)
		if merr != nil {
			f.Error = &FrameError{Code: engine.PortUnavailable, Message: "port result is not serializable"}
		} else {
			f.Result = raw
		}
	}
	if werr := h.out.send(f); werr != nil {
		h.spec.Log.Debug("script host reply failed", "err", werr)
	}
}

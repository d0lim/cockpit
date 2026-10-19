package dockerport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/ports"
)

const (
	defaultMaxBytes   = 1 << 20
	defaultMaxEntries = 64
	defaultTimeout    = 10 * time.Second
	defaultMaxTail    = 10000
	maskMargin        = 4096
	shellChars        = " \t\n;|&$`<>(){}*?"
)

var shells = map[string]bool{"sh": true, "bash": true, "dash": true, "ash": true, "zsh": true, "ksh": true, "csh": true, "tcsh": true, "fish": true, "busybox": true, "env": true}

type ExecAudit struct {
	At       time.Time `json:"at"`
	Service  string    `json:"service"`
	Argv     []string  `json:"argv"`
	Outcome  string    `json:"outcome"`
	Code     string    `json:"code,omitempty"`
	ExitCode int       `json:"exitCode"`
}

type Options struct {
	Project    string
	RunID      string
	Builder    string
	Docker     compose.Docker
	Allow      [][]string
	Mask       ports.Mask
	Audit      func(ExecAudit)
	MaxBytes   int
	MaxEntries int
	Timeout    time.Duration
	Now        func() time.Time
}

type Port struct {
	project, runID string
	harness        map[string]bool
	docker         compose.Docker
	allow          [][]string
	mask           ports.Mask
	audit          func(ExecAudit)
	maxBytes       int
	maxEntries     int
	timeout        time.Duration
	now            func() time.Time
}

type TextOutput struct {
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

func New(opts Options) (*Port, error) {
	if opts.Project == "" || opts.RunID == "" || opts.Docker == nil {
		return nil, errors.New("the docker port needs a compose project, a run id and a docker client")
	}
	p := &Port{project: opts.Project, runID: opts.RunID, docker: opts.Docker, mask: opts.Mask, audit: opts.Audit,
		maxBytes: opts.MaxBytes, maxEntries: opts.MaxEntries, timeout: opts.Timeout, now: opts.Now, harness: map[string]bool{}}
	for _, role := range append([]string{compose.RoleBuilder}, compose.HarnessServices...) {
		p.harness[role] = true
	}
	if opts.Builder != "" {
		p.harness[opts.Builder] = true
	}
	for i, prefix := range opts.Allow {
		if len(prefix) == 0 || shells[path.Base(prefix[0])] || strings.ContainsAny(prefix[0], shellChars) {
			return nil, fmt.Errorf("exec.allow[%d] must start with a program name that is not a shell or a shell string", i)
		}
		p.allow = append(p.allow, append([]string(nil), prefix...))
	}
	if p.mask == nil {
		p.mask = ports.Identity
	}
	if p.audit == nil {
		p.audit = func(a ExecAudit) {
			slog.Info("docker exec", "service", a.Service, "argv", a.Argv, "outcome", a.Outcome, "code", a.Code, "exitCode", a.ExitCode)
		}
	}
	if p.maxBytes <= 0 {
		p.maxBytes = defaultMaxBytes
	}
	if p.maxEntries <= 0 {
		p.maxEntries = defaultMaxEntries
	}
	if p.timeout <= 0 {
		p.timeout = defaultTimeout
	}
	if p.now == nil {
		p.now = time.Now
	}
	return p, nil
}

func (p *Port) bytesFor(requested int) int {
	if requested > 0 && requested < p.maxBytes {
		return requested
	}
	return p.maxBytes
}

func (p *Port) timeoutFor(ms int64) time.Duration {
	if t := time.Duration(ms) * time.Millisecond; t > 0 && t < p.timeout {
		return t
	}
	return p.timeout
}

func (p *Port) failure(ctx context.Context, err error, res compose.Result, timeout time.Duration, at string) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ports.Timeout(p.mask, fmt.Sprintf("%s exceeded the %dms time limit", at, timeout.Milliseconds()))
	}
	if err != nil {
		return ports.Unavailable(p.mask, at+" could not run")
	}
	return ports.Unavailable(p.mask, fmt.Sprintf("%s failed (exit %d): %s", at, res.Code, clipText(p.mask(string(res.Stderr)), 512)))
}

type capWriter struct {
	mu    sync.Mutex
	buf   []byte
	limit int
	over  bool
	full  func()
}

func newCap(limit int, full func()) *capWriter {
	return &capWriter{limit: limit, full: full}
}

func (w *capWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	room := w.limit - len(w.buf)
	if room >= len(b) {
		w.buf = append(w.buf, b...)
		return len(b), nil
	}
	if room > 0 {
		w.buf = append(w.buf, b[:room]...)
	}
	if !w.over {
		w.over = true
		if w.full != nil {
			w.full()
		}
	}
	return len(b), nil
}

func (w *capWriter) result() ([]byte, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buf...), w.over
}

func rawLimit(max int) int { return max + maskMargin }

func clip(raw []byte, over bool, max int, mask ports.Mask) TextOutput {
	text := mask(string(raw))
	if len(text) > max {
		return TextOutput{Text: clipText(text, max), Truncated: true}
	}
	return TextOutput{Text: text, Truncated: over}
}

func clipText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

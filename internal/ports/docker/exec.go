package dockerport

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports"
)

const (
	maxArgs   = 256
	maxArgLen = 4096
)

type ExecRequest struct {
	Service   string   `json:"service"`
	Argv      []string `json:"argv"`
	TimeoutMs int64    `json:"timeoutMs,omitempty"`
	MaxBytes  int      `json:"maxBytes,omitempty"`
}

type ExecOutput struct {
	ExitCode  int    `json:"exitCode"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Truncated bool   `json:"truncated"`
}

func (p *Port) Exec(ctx context.Context, req ExecRequest) (out ExecOutput, err error) {
	audit := ExecAudit{At: p.now().UTC(), Service: p.mask(req.Service), Argv: make([]string, len(req.Argv))}
	for i, a := range req.Argv {
		audit.Argv[i] = p.mask(a)
	}
	defer func() {
		audit.Outcome, audit.ExitCode = "allowed", out.ExitCode
		var pe *engine.PortError
		if errors.As(err, &pe) {
			audit.Outcome, audit.Code = "failed", string(pe.Code)
			if pe.Code == engine.PortDenied || pe.Code == engine.PortBadArgs {
				audit.Outcome = "denied"
			}
		}
		p.audit(audit)
	}()
	if err := p.checkArgv(req.Argv); err != nil {
		return ExecOutput{}, err
	}
	id, err := p.resolve(ctx, req.Service)
	if err != nil {
		return ExecOutput{}, err
	}
	max := p.bytesFor(req.MaxBytes)
	timeout := p.timeoutFor(req.TimeoutMs)
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stop, halt := context.WithCancel(cctx)
	defer halt()
	stdout, stderr := newCap(rawLimit(max), halt), newCap(rawLimit(max), halt)
	args := append([]string{"exec", id}, req.Argv...)
	res, runErr := p.docker.Run(stop, compose.Command{Args: args, Stdout: stdout, Stderr: stderr})
	rawOut, overOut := stdout.result()
	rawErr, overErr := stderr.result()
	if !overOut && !overErr && (runErr != nil || errors.Is(cctx.Err(), context.DeadlineExceeded)) {
		return ExecOutput{}, p.failure(cctx, runErr, res, timeout, "docker exec "+req.Service)
	}
	o, e := clip(rawOut, overOut, max, p.mask), clip(rawErr, overErr, max, p.mask)
	exit := res.Code
	if overOut || overErr {
		exit = -1
	}
	return ExecOutput{ExitCode: exit, Stdout: o.Text, Stderr: e.Text, Truncated: o.Truncated || e.Truncated}, nil
}

func (p *Port) checkArgv(argv []string) error {
	if len(argv) == 0 || len(argv) > maxArgs {
		return ports.BadArgs(p.mask, "argv must hold between 1 and 256 elements")
	}
	for _, a := range argv {
		if len(a) > maxArgLen || strings.ContainsRune(a, 0) {
			return ports.BadArgs(p.mask, "an argv element is too long or holds a NUL byte")
		}
	}
	for _, prefix := range p.allow {
		if len(argv) >= len(prefix) && slices.Equal(argv[:len(prefix)], prefix) {
			return nil
		}
	}
	return ports.Denied(p.mask, "argv is not allowed by exec.allow in the project declaration")
}

package seal

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/d0lim/cockpit/internal/compose"
)

type attempt struct {
	key   string
	label string
}

var attempts = []attempt{
	{"tcp", "external IP TCP 1.1.1.1:443"},
	{"dns", "external hostname DNS example.com"},
	{"host", "host canary host.docker.internal"},
	{"udp", "external UDP 1.1.1.1:53"},
}

const hostAttempt = "host"

var probeLine = regexp.MustCompile(`^probe=(\d+) rc=(\d+)$`)

type ProbeRun struct {
	Code  int
	Lines []string
}

type RunProbe func(ctx context.Context, service string) (ProbeRun, error)

type Canary struct {
	ln       net.Listener
	accepted atomic.Int64
	done     chan struct{}
}

func ListenCanary(port int) (*Canary, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("cannot open the seal canary on 127.0.0.1:%d: %w", port, err)
	}
	c := &Canary{ln: ln, done: make(chan struct{})}
	go func() {
		defer close(c.done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			c.accepted.Add(1)
			conn.Close()
		}
	}()
	return c, nil
}

func (c *Canary) Accepted() int { return int(c.accepted.Load()) }

func (c *Canary) Port() int { return c.ln.Addr().(*net.TCPAddr).Port }

func (c *Canary) Close() error {
	err := c.ln.Close()
	<-c.done
	return err
}

func Probes(ctx context.Context, run RunProbe, canary interface{ Accepted() int }, canaryPort int) []Result {
	sealed, sealedErr := run(ctx, compose.RoleProbe)
	duringSealed := canary.Accepted()
	control, controlErr := run(ctx, compose.RoleProbeControl)
	duringControl := canary.Accepted() - duringSealed
	out := make([]Result, 0, 2*len(attempts))
	for i, a := range attempts {
		out = append(out, judgeSealed(a, codesOf(sealed, i+1), sealed.Code, sealedErr, canaryPort, duringSealed))
	}
	for i, a := range attempts {
		out = append(out, judgeControl(a, codesOf(control, i+1), control.Code, controlErr, canaryPort, duringControl))
	}
	return out
}

func codesOf(run ProbeRun, n int) []int {
	var codes []int
	for _, l := range run.Lines {
		m := probeLine.FindStringSubmatch(strings.TrimSpace(l))
		if m != nil && m[1] == strconv.Itoa(n) {
			rc, _ := strconv.Atoi(m[2])
			codes = append(codes, rc)
		}
	}
	return codes
}

func probeCommand(a attempt, port int) string {
	return fmt.Sprintf("cockpit-runner probe %d (%s)", port, a.label)
}

func noLines(code int, err error) string {
	if err != nil {
		return fmt.Sprintf("no result line (%v)", err)
	}
	return fmt.Sprintf("no result line (compose run exit code %d)", code)
}

func joinCodes(codes []int) string {
	parts := make([]string, len(codes))
	for i, c := range codes {
		parts[i] = strconv.Itoa(c)
	}
	return strings.Join(parts, ",")
}

func judgeSealed(a attempt, codes []int, code int, err error, port, knocks int) Result {
	host := a.key == hostAttempt
	r := Result{Name: "probe." + a.key, Command: probeCommand(a, port), Expected: "fails (rc != 0)"}
	if host {
		r.Expected = "fails (rc != 0) and the canary accepts no connection"
	}
	switch {
	case containsZero(codes):
		r.Detail = "rc=0: the sealed network reached " + a.label
	case host && knocks > 0:
		r.Detail = fmt.Sprintf("the canary accepted %d connections: the sealed network reached the host", knocks)
	case len(codes) == 0:
		r.Inconclusive, r.Detail = true, "inconclusive: "+noLines(code, err)
	default:
		r.OK, r.Detail = true, "rc="+joinCodes(codes)
		if host {
			r.Detail += ", canary accepted 0"
		}
	}
	return r
}

func judgeControl(a attempt, codes []int, code int, err error, port, knocks int) Result {
	host := a.key == hostAttempt
	r := Result{Name: "control." + a.key, Command: probeCommand(a, port), Expected: "succeeds (rc=0)"}
	if host {
		r.Expected = "succeeds (rc=0) and the canary accepts exactly one connection"
	}
	why := ""
	switch {
	case len(codes) == 0:
		why = noLines(code, err)
	case !allZero(codes):
		why = "rc=" + joinCodes(codes)
	case host && knocks != 1:
		why = fmt.Sprintf("the canary accepted %d connections", knocks)
	}
	if why != "" {
		r.Inconclusive, r.Detail = true, "no control: "+why
		return r
	}
	r.OK, r.Detail = true, "rc=0"
	if host {
		r.Detail += ", canary accepted 1"
	}
	return r
}

func containsZero(codes []int) bool {
	for _, c := range codes {
		if c == 0 {
			return true
		}
	}
	return false
}

func allZero(codes []int) bool {
	for _, c := range codes {
		if c != 0 {
			return false
		}
	}
	return true
}

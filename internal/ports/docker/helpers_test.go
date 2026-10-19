package dockerport

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/engine"
)

const (
	planted = "PLANTED-secret-5e8a"
	project = "cockpit-run-1"
	runID   = "run-1"
)

type fakeDocker struct {
	mu     sync.Mutex
	calls  [][]string
	ps     []string
	handle func(ctx context.Context, cmd compose.Command) (compose.Result, error)
}

func (f *fakeDocker) Run(ctx context.Context, cmd compose.Command) (compose.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), cmd.Args...))
	f.mu.Unlock()
	if cmd.Args[0] == "ps" {
		return compose.Result{Stdout: []byte(strings.Join(f.ps, "\n") + "\n")}, nil
	}
	if f.handle == nil {
		return compose.Result{}, errors.New("unexpected docker call")
	}
	return f.handle(ctx, cmd)
}

func (f *fakeDocker) commands(verb string) [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out [][]string
	for _, c := range f.calls {
		if c[0] == verb {
			out = append(out, c)
		}
	}
	return out
}

func psLine(id, service, state, status, proj, run, role string) string {
	return strings.Join([]string{id, proj + "-" + service + "-1", state, status, proj, service, run, role}, "\t")
}

func standardPS() []string {
	return []string{
		psLine("c-app", "app", "running", "Up 2 minutes (healthy)", project, runID, ""),
		psLine("c-db", "db", "running", "Up 2 minutes (health: starting)", project, runID, ""),
		psLine("c-worker", "worker", "exited", "Exited (0) 1 minute ago", project, runID, ""),
		psLine("c-edge", "edge", "running", "Up", project, runID, compose.RoleEdge),
		psLine("c-probe", "probe", "exited", "Exited (0)", project, runID, compose.RoleProbe),
		psLine("c-toolbox", "toolbox", "exited", "Exited (0)", project, runID, compose.RoleBuilder),
		psLine("c-sneaky", "sneaky", "running", "Up", project, runID, compose.RoleRunner),
		psLine("c-intruder", "intruder", "running", "Up", "other-project", "run-2", ""),
		psLine("c-stale", "stale", "running", "Up", project, "run-0", ""),
	}
}

type audits struct {
	mu     sync.Mutex
	events []ExecAudit
}

func (a *audits) record(e ExecAudit) {
	a.mu.Lock()
	a.events = append(a.events, e)
	a.mu.Unlock()
}

func newPort(t *testing.T, f *fakeDocker, opts Options) (*Port, *audits) {
	t.Helper()
	if f.ps == nil {
		f.ps = standardPS()
	}
	a := &audits{}
	opts.Project, opts.RunID, opts.Docker, opts.Builder = project, runID, f, "toolbox"
	if opts.Mask == nil {
		opts.Mask = func(s string) string { return strings.ReplaceAll(s, planted, "****") }
	}
	if opts.Audit == nil {
		opts.Audit = a.record
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Date(2026, 10, 19, 1, 2, 3, 0, time.UTC) }
	}
	p, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return p, a
}

func write(cmd compose.Command, stdout, stderr string) {
	if stdout != "" && cmd.Stdout != nil {
		_, _ = cmd.Stdout.Write([]byte(stdout))
	}
	if stderr != "" && cmd.Stderr != nil {
		_, _ = cmd.Stderr.Write([]byte(stderr))
	}
}

type tarEntry struct {
	name, body, link string
	kind             byte
}

func tarball(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Typeflag: e.kind, Mode: 0o644, Size: int64(len(e.body)), Linkname: e.link}
		if e.kind != tar.TypeReg {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if e.kind == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func codeOf(err error) engine.PortCode {
	var pe *engine.PortError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

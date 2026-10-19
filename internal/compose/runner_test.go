package compose

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var runnerMutations = []porttest.Mutation{
	{"runner lines pass the run redactor", "build Lines from raw logs", "red: the planted value appears in the runner lines", "TestRunnerRunsDetachedThenWaitsReadsLogsChecksOOMAndRemovesInThatOrder"},
	{"a runner over its limit is logged and force-removed", "skip the timeout branch", "red: no rm -f after the limit and TimedOut stays false", "TestRunnerOverItsLimitIsLoggedAndForceRemovedWithoutAnExitCode"},
	{"a runner start without a container id fails and clears one-off containers", "accept any last stdout line as the id", "red: docker wait runs with a bogus id", "TestRunnerStartWithoutAContainerIDFailsAndRemovesTheRunnerOneOffs"},
}

func TestRunnerMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "runner_test.go", runnerMutations)
}

const fakeContainer = "0123456789abcdef0123"

type scripted struct {
	calls [][]string
	envs  [][]string
	reply func(ctx context.Context, args []string) (Result, error)
}

func (s *scripted) Run(ctx context.Context, cmd Command) (Result, error) {
	s.calls = append(s.calls, cmd.Args)
	s.envs = append(s.envs, cmd.Env)
	return s.reply(ctx, cmd.Args)
}

func verbs(calls [][]string) []string {
	var out []string
	for _, c := range calls {
		if c[0] == "compose" {
			i := slices.Index(c, "run")
			if i < 0 {
				i = len(c) - 1
			}
			out = append(out, "compose "+c[i])
			continue
		}
		out = append(out, strings.Join(c[:min(len(c), 2)], " "))
	}
	return out
}

func runnerProject(t *testing.T, d Docker) Project {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	return Project{Docker: d, Name: "r-toy", Args: []string{"compose", "-p", "r-toy"}, Mask: m.Mask}
}

func normalRunner(code, oom string) *scripted {
	return &scripted{reply: func(ctx context.Context, args []string) (Result, error) {
		switch args[0] {
		case "compose":
			return Result{Stdout: []byte("Creating\n" + fakeContainer + "\n")}, nil
		case "wait":
			return Result{Stdout: []byte(code + "\n")}, nil
		case "logs":
			return Result{Stdout: []byte("health › ok passed\ntoken " + planted + "\n"), Stderr: []byte("warn line\n")}, nil
		case "inspect":
			return Result{Stdout: []byte(oom + "\n")}, nil
		}
		return Result{}, nil
	}}
}

func TestRunnerRunsDetachedThenWaitsReadsLogsChecksOOMAndRemovesInThatOrder(t *testing.T) {
	d := normalRunner("1", "false")
	r, err := runnerProject(t, d).RunRunner(context.Background(), []string{"run", "smoke"}, []string{"E2E_EXEC_OUT=/work/e2e/exec/x.json"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(verbs(d.calls), " > "); got != "compose run > wait "+fakeContainer+" > logs "+fakeContainer+" > inspect --format > rm "+fakeContainer {
		t.Fatalf("sequence %s", got)
	}
	start := d.calls[0]
	if !slices.Equal(start[3:], []string{"--profile", "runner", "run", "-d", "-T", "--no-deps", "runner", "run", "smoke"}) {
		t.Fatalf("start %q", start)
	}
	if !slices.Contains(d.envs[0], "E2E_EXEC_OUT=/work/e2e/exec/x.json") {
		t.Fatalf("start env %q", d.envs[0])
	}
	if r.Code == nil || *r.Code != 1 || r.OOMKilled || r.TimedOut || r.ContainerID != fakeContainer || len(r.Lines) != 3 {
		t.Fatalf("result %+v", r)
	}
	text := strings.Join(r.Lines, "\n")
	if strings.Contains(text, planted) || !strings.Contains(text, "****") {
		t.Fatalf("lines %q", r.Lines)
	}
	oom := normalRunner("137", "true")
	r, err = runnerProject(t, oom).RunRunner(context.Background(), []string{"run", "smoke"}, nil, time.Minute)
	if err != nil || !r.OOMKilled || *r.Code != 137 {
		t.Fatalf("oom %+v %v", r, err)
	}
}

func TestRunnerOverItsLimitIsLoggedAndForceRemovedWithoutAnExitCode(t *testing.T) {
	d := normalRunner("0", "false")
	inner := d.reply
	d.reply = func(ctx context.Context, args []string) (Result, error) {
		if args[0] == "wait" {
			<-ctx.Done()
			return Result{Code: -1}, ctx.Err()
		}
		return inner(ctx, args)
	}
	r, err := runnerProject(t, d).RunRunner(context.Background(), []string{"run", "smoke"}, nil, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !r.TimedOut || r.Aborted || r.Code != nil || len(r.Lines) == 0 {
		t.Fatalf("result %+v", r)
	}
	last := d.calls[len(d.calls)-1]
	if !slices.Equal(last, []string{"rm", "-f", fakeContainer}) || slices.ContainsFunc(d.calls, func(c []string) bool { return c[0] == "inspect" }) {
		t.Fatalf("calls %q", d.calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.calls = nil
	go func() { time.Sleep(10 * time.Millisecond); cancel() }()
	r, err = runnerProject(t, d).RunRunner(ctx, []string{"run", "smoke"}, nil, time.Minute)
	if err != nil || !r.Aborted || r.TimedOut {
		t.Fatalf("aborted %+v %v", r, err)
	}
}

func TestRunnerStartWithoutAContainerIDFailsAndRemovesTheRunnerOneOffs(t *testing.T) {
	d := &scripted{reply: func(ctx context.Context, args []string) (Result, error) {
		if args[0] == "compose" {
			return Result{Stdout: []byte("Creating\nnot-an-id\n"), Stderr: []byte("token " + planted)}, nil
		}
		return Result{}, nil
	}}
	_, err := runnerProject(t, d).RunRunner(context.Background(), []string{"run", "smoke"}, nil, time.Minute)
	if err == nil || strings.Contains(err.Error(), planted) {
		t.Fatalf("err %v", err)
	}
	if slices.ContainsFunc(d.calls, func(c []string) bool { return c[0] == "wait" }) {
		t.Fatal("docker wait ran without a container id")
	}
	ps := d.calls[1]
	if ps[0] != "ps" || !slices.Contains(ps, "label=com.docker.compose.service=runner") {
		t.Fatalf("one-off cleanup %q", ps)
	}
}

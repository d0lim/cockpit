package dockerport

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var execMutations = []porttest.Mutation{
	{"only declared argv prefixes run", "return nil at the end of checkArgv", "red: valkey-cli FLUSHALL runs", "TestAllowedPrefixRunsAndOtherArgvShellsAndHarnessTargetsArePortDenied"},
	{"harness containers are not exec targets", "skip resolve and exec into the service name", "red: exec into edge or probe runs", "TestAllowedPrefixRunsAndOtherArgvShellsAndHarnessTargetsArePortDenied"},
	{"every call leaves exactly one audit event", "call audit only on success", "red: denied calls leave no audit event", "TestEveryExecCallLeavesExactlyOneAuditEventWithMaskedArgv"},
	{"audit argv is masked", "store the raw argv in the audit event", "red: the planted value shows in the audit argv", "TestEveryExecCallLeavesExactlyOneAuditEventWithMaskedArgv"},
	{"exec output limit", "return stdout and stderr without clip", "red: the whole output comes back", "TestExecOutputOverTheLimitIsTruncatedAndExceedingTheTimeLimitIsPortTimeout"},
	{"exec time limit", "replace WithTimeout with WithCancel in Exec", "red: the blocked call never returns", "TestExecOutputOverTheLimitIsTruncatedAndExceedingTheTimeLimitIsPortTimeout"},
	{"the allow list cannot name a shell", "drop the shells lookup in New", "red: New accepts an allow prefix that starts with sh or bash", "TestAllowListsThatStartWithAShellOrAShellStringAreRejectedAtConstruction"},
	{"the allow list cannot start with a shell string", "drop the shellChars check in New", "red: New accepts an allow prefix whose program holds spaces", "TestAllowListsThatStartWithAShellOrAShellStringAreRejectedAtConstruction"},
}

var allow = [][]string{{"/opt/kafka/bin/kafka-console-consumer.sh"}, {"valkey-cli", "--scan"}, {"valkey-cli", "GET"}}

func echoExec(_ context.Context, cmd compose.Command) (compose.Result, error) {
	write(cmd, "out:"+strings.Join(cmd.Args[2:], " "), "warn")
	return compose.Result{Code: 0}, nil
}

func TestAllowedPrefixRunsAndOtherArgvShellsAndHarnessTargetsArePortDenied(t *testing.T) {
	f := &fakeDocker{handle: echoExec}
	p, _ := newPort(t, f, Options{Allow: allow})
	argv := []string{"/opt/kafka/bin/kafka-console-consumer.sh", "--bootstrap-server", "kafka:9092", "--property", "key.separator=|"}
	out, err := p.Exec(context.Background(), ExecRequest{Service: "app", Argv: argv})
	if err != nil || out.ExitCode != 0 || out.Stdout != "out:"+strings.Join(argv, " ") || out.Stderr != "warn" || out.Truncated {
		t.Fatalf("%+v %v", out, err)
	}
	if _, err := p.Exec(context.Background(), ExecRequest{Service: "app", Argv: []string{"valkey-cli", "GET", "k"}}); err != nil {
		t.Fatalf("second prefix: %v", err)
	}
	denied := []ExecRequest{
		{Service: "app", Argv: []string{"valkey-cli", "FLUSHALL"}},
		{Service: "app", Argv: []string{"valkey-cli"}},
		{Service: "app", Argv: []string{"sh", "-c", "valkey-cli GET k"}},
		{Service: "app", Argv: []string{"/bin/bash", "-c", "id"}},
		{Service: "app", Argv: []string{"env", "valkey-cli", "GET", "k"}},
		{Service: "app", Argv: []string{"valkey-cli GET k"}},
		{Service: "app", Argv: []string{"valkey-cli;FLUSHALL"}},
		{Service: "edge", Argv: []string{"valkey-cli", "GET", "k"}},
		{Service: "probe", Argv: []string{"valkey-cli", "GET", "k"}},
		{Service: "intruder", Argv: []string{"valkey-cli", "GET", "k"}},
	}
	for _, r := range denied {
		if _, err := p.Exec(context.Background(), r); codeOf(err) != engine.PortDenied {
			t.Errorf("%s %q: %v", r.Service, r.Argv, err)
		}
	}
	execs := f.commands("exec")
	if len(execs) != 2 || !slices.Equal(execs[0], append([]string{"exec", "c-app"}, argv...)) {
		t.Fatalf("docker exec calls after %d denials: %v", len(denied), execs)
	}
}

func TestEveryExecCallLeavesExactlyOneAuditEventWithMaskedArgv(t *testing.T) {
	f := &fakeDocker{handle: echoExec}
	p, a := newPort(t, f, Options{Allow: allow})
	calls := []ExecRequest{
		{Service: "app", Argv: []string{"valkey-cli", "GET", planted}},
		{Service: "app", Argv: []string{"valkey-cli", "FLUSHALL", planted}},
		{Service: "edge", Argv: []string{"valkey-cli", "GET", "k"}},
		{Service: "app", Argv: nil},
	}
	for _, c := range calls {
		_, _ = p.Exec(context.Background(), c)
	}
	if len(a.events) != len(calls) || len(calls) != 4 {
		t.Fatalf("%d audit events for %d calls", len(a.events), len(calls))
	}
	outcomes := []string{a.events[0].Outcome, a.events[1].Outcome, a.events[2].Outcome, a.events[3].Outcome}
	if !slices.Equal(outcomes, []string{"allowed", "denied", "denied", "denied"}) || a.events[1].Code != "PORT_DENIED" || a.events[3].Code != "PORT_BAD_ARGS" {
		t.Fatalf("%+v", a.events)
	}
	first := a.events[0]
	if first.Service != "app" || !slices.Equal(first.Argv, []string{"valkey-cli", "GET", "****"}) || slices.Contains(a.events[1].Argv, planted) || !first.At.Equal(time.Date(2026, 10, 19, 1, 2, 3, 0, time.UTC)) {
		t.Fatalf("%+v", a.events)
	}
}

func TestExecOutputOverTheLimitIsTruncatedAndExceedingTheTimeLimitIsPortTimeout(t *testing.T) {
	f := &fakeDocker{handle: func(_ context.Context, cmd compose.Command) (compose.Result, error) {
		write(cmd, strings.Repeat("k", 301+maskMargin), "")
		return compose.Result{}, nil
	}}
	p, a := newPort(t, f, Options{Allow: allow, Timeout: time.Second})
	out, err := p.Exec(context.Background(), ExecRequest{Service: "app", Argv: []string{"valkey-cli", "--scan"}, MaxBytes: 300})
	if err != nil || len(out.Stdout) != 300 || !out.Truncated || out.ExitCode != -1 {
		t.Fatalf("over: %d %+v %v", len(out.Stdout), out.Truncated, err)
	}
	f.handle = func(_ context.Context, cmd compose.Command) (compose.Result, error) {
		write(cmd, strings.Repeat("k", 300), "")
		return compose.Result{Code: 3}, nil
	}
	out, err = p.Exec(context.Background(), ExecRequest{Service: "app", Argv: []string{"valkey-cli", "--scan"}, MaxBytes: 300})
	if err != nil || len(out.Stdout) != 300 || out.Truncated || out.ExitCode != 3 {
		t.Fatalf("at the limit: %d %+v %v", len(out.Stdout), out, err)
	}
	f.handle = func(ctx context.Context, cmd compose.Command) (compose.Result, error) {
		<-ctx.Done()
		return compose.Result{}, ctx.Err()
	}
	_, err = p.Exec(context.Background(), ExecRequest{Service: "app", Argv: []string{"valkey-cli", "--scan"}, TimeoutMs: 30})
	if codeOf(err) != engine.PortTimeout || !strings.Contains(err.Error(), "30ms") {
		t.Fatalf("%v", err)
	}
	if last := a.events[len(a.events)-1]; last.Outcome != "failed" || last.Code != "PORT_TIMEOUT" {
		t.Fatalf("%+v", last)
	}
}

func TestAllowListsThatStartWithAShellOrAShellStringAreRejectedAtConstruction(t *testing.T) {
	bad := [][][]string{{{"sh", "-c"}}, {{"/bin/bash"}}, {{"valkey-cli GET"}}, {{"tool;id"}}, {{}}}
	for _, bad := range bad {
		if _, err := New(Options{Project: project, RunID: runID, Docker: &fakeDocker{}, Allow: bad}); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	if len(bad) != 5 {
		t.Fatalf("ran %d cases", len(bad))
	}
	if _, err := New(Options{Project: project, RunID: runID, Docker: &fakeDocker{}, Allow: allow}); err != nil {
		t.Fatal(err)
	}
}

func TestExecMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "exec_test.go", execMutations)
}

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

var logsMutations = []porttest.Mutation{
	{"log byte limit", "disable the length cut in clip", "red: 300 bytes come back with truncated false", "TestLogsOverTheByteLimitAreTruncatedAndLogsWithinItAreNot"},
	{"stderr lines are kept with stdout", "pass no Stderr writer to docker logs", "red: the stderr line is missing", "TestLogsOverTheByteLimitAreTruncatedAndLogsWithinItAreNot"},
	{"since and tail are checked before docker runs", "pass since and tail through unchecked", "red: --since=--all reaches docker", "TestLogsPassSinceAndTailAndRejectValuesThatAreNotDurationsOrTimes"},
	{"logs and stats output pass the masker before truncation", "drop mask in clip and in Stats", "red: the planted value shows in the output", "TestLogsAndStatsOutputPassTheMaskerBeforeTruncation"},
	{"stats is one snapshot", "drop --no-stream", "red: the stats call lacks --no-stream", "TestStatsReadsOneSnapshotWithoutStreaming"},
	{"time limit", "remove the WithTimeout around docker logs", "red: the blocked call never returns", "TestLogsExceedingTheTimeLimitArePortTimeout"},
}

func TestLogsOverTheByteLimitAreTruncatedAndLogsWithinItAreNot(t *testing.T) {
	body := strings.Repeat("o", 200)
	f := &fakeDocker{handle: func(_ context.Context, cmd compose.Command) (compose.Result, error) {
		write(cmd, body, "err-line\n")
		return compose.Result{}, nil
	}}
	p, _ := newPort(t, f, Options{})
	out, err := p.Logs(context.Background(), LogsRequest{Service: "app", MaxBytes: 100})
	if err != nil || len(out.Text) != 100 || !out.Truncated {
		t.Fatalf("over: %d %v %v", len(out.Text), out.Truncated, err)
	}
	out, err = p.Logs(context.Background(), LogsRequest{Service: "app", MaxBytes: 209})
	if err != nil || out.Text != body+"err-line\n" || out.Truncated {
		t.Fatalf("at the limit: %d %v %v", len(out.Text), out.Truncated, err)
	}
	big := strings.Repeat("b", 300+maskMargin)
	f.handle = func(_ context.Context, cmd compose.Command) (compose.Result, error) {
		write(cmd, big, "")
		return compose.Result{}, nil
	}
	if out, err := p.Logs(context.Background(), LogsRequest{Service: "app", MaxBytes: 300}); err != nil || len(out.Text) != 300 || !out.Truncated {
		t.Fatalf("beyond the read margin: %d %v %v", len(out.Text), out.Truncated, err)
	}
}

func TestLogsPassSinceAndTailAndRejectValuesThatAreNotDurationsOrTimes(t *testing.T) {
	f := &fakeDocker{handle: func(context.Context, compose.Command) (compose.Result, error) { return compose.Result{}, nil }}
	p, _ := newPort(t, f, Options{})
	tail := 50
	for _, since := range []string{"10m", "1h30m", "2026-10-19T01:02:03Z", "2026-10-19", "1760835723"} {
		if _, err := p.Logs(context.Background(), LogsRequest{Service: "app", Since: since, Tail: &tail}); err != nil {
			t.Fatalf("%s: %v", since, err)
		}
	}
	logs := f.commands("logs")
	if len(logs) != 5 || !slices.Equal(logs[0], []string{"logs", "--since=10m", "--tail=50", "c-app"}) {
		t.Fatalf("%v", logs)
	}
	negative, huge := -1, defaultMaxTail+1
	bad := []LogsRequest{{Service: "app", Since: "--all"}, {Service: "app", Since: "10m; rm"}, {Service: "app", Since: "yesterday"}, {Service: "app", Tail: &negative}, {Service: "app", Tail: &huge}}
	for _, r := range bad {
		if _, err := p.Logs(context.Background(), r); codeOf(err) != engine.PortBadArgs {
			t.Errorf("%+v: %v", r, err)
		}
	}
	if n := len(f.commands("logs")); n != 5 {
		t.Fatalf("%d logs calls after %d rejected requests", n, len(bad))
	}
}

func TestLogsAndStatsOutputPassTheMaskerBeforeTruncation(t *testing.T) {
	f := &fakeDocker{handle: func(_ context.Context, cmd compose.Command) (compose.Result, error) {
		if cmd.Args[0] == "stats" {
			write(cmd, `{"Name":"app","MemUsage":"`+planted+`","PIDs":"3"}`+"\n", "")
			return compose.Result{}, nil
		}
		write(cmd, "token="+planted+" ok\n", "")
		return compose.Result{}, nil
	}}
	p, _ := newPort(t, f, Options{})
	out, err := p.Logs(context.Background(), LogsRequest{Service: "app"})
	if err != nil || strings.Contains(out.Text, planted) || out.Text != "token=**** ok\n" {
		t.Fatalf("%+v %v", out, err)
	}
	out, err = p.Logs(context.Background(), LogsRequest{Service: "app", MaxBytes: 12})
	if err != nil || out.Text != "token=**** o" || !out.Truncated || strings.Contains(out.Text, "PLANTED") {
		t.Fatalf("masked before cut: %q %v", out.Text, err)
	}
	stats, err := p.Stats(context.Background(), "app")
	if err != nil || stats["MemUsage"] != "****" || stats["PIDs"] != "3" {
		t.Fatalf("%v %v", stats, err)
	}
}

func TestStatsReadsOneSnapshotWithoutStreaming(t *testing.T) {
	f := &fakeDocker{handle: func(_ context.Context, cmd compose.Command) (compose.Result, error) {
		write(cmd, `{"CPUPerc":"0.5%","MemUsage":"10MiB / 1GiB"}`, "")
		return compose.Result{}, nil
	}}
	p, _ := newPort(t, f, Options{})
	stats, err := p.Stats(context.Background(), "app")
	if err != nil || stats["CPUPerc"] != "0.5%" || len(stats) != 2 {
		t.Fatalf("%v %v", stats, err)
	}
	calls := f.commands("stats")
	if len(calls) != 1 || !slices.Contains(calls[0], "--no-stream") || calls[0][len(calls[0])-1] != "c-app" {
		t.Fatalf("%v", calls)
	}
	if _, err := p.Stats(context.Background(), "edge"); codeOf(err) != engine.PortDenied {
		t.Fatalf("harness stats: %v", err)
	}
}

func TestLogsExceedingTheTimeLimitArePortTimeout(t *testing.T) {
	f := &fakeDocker{handle: func(ctx context.Context, cmd compose.Command) (compose.Result, error) {
		<-ctx.Done()
		return compose.Result{}, ctx.Err()
	}}
	p, _ := newPort(t, f, Options{Timeout: 40 * time.Millisecond})
	_, err := p.Logs(context.Background(), LogsRequest{Service: "app"})
	if codeOf(err) != engine.PortTimeout || !strings.Contains(err.Error(), "40ms") {
		t.Fatalf("%v", err)
	}
	f.handle = func(_ context.Context, cmd compose.Command) (compose.Result, error) {
		write(cmd, "fast\n", "")
		return compose.Result{}, nil
	}
	if out, err := p.Logs(context.Background(), LogsRequest{Service: "app"}); err != nil || out.Text != "fast\n" {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestLogsMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "logs_test.go", logsMutations)
}

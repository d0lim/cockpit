package dockerport

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func bufferLogger() (*slog.Logger, *bytes.Buffer) {
	var b bytes.Buffer
	return slog.New(slog.NewTextHandler(&b, nil)), &b
}

func TestTheDefaultExecAuditWritesTheRunAndMaskedArgvToTheInjectedLoggerAndNothingToTheProcessDefault(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	fallback, fallbackOut := bufferLogger()
	slog.SetDefault(fallback)
	injected, out := bufferLogger()
	p, err := New(Options{Project: project, RunID: runID, Docker: &fakeDocker{handle: echoExec, ps: standardPS()}, Allow: allow, Log: injected,
		Mask: func(s string) string { return strings.ReplaceAll(s, planted, "****") }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(context.Background(), ExecRequest{Service: "app", Argv: []string{"valkey-cli", "GET", planted}}); err != nil {
		t.Fatal(err)
	}
	_, _ = p.Exec(context.Background(), ExecRequest{Service: "app", Argv: []string{"valkey-cli", "FLUSHALL"}})
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `msg="docker exec" run=`+runID+` service=app`) || !strings.Contains(lines[0], "****") || !strings.Contains(lines[1], "outcome=denied") {
		t.Fatalf("injected logger lines:\n%s", out.String())
	}
	if strings.Contains(out.String(), planted) {
		t.Fatalf("the audit line holds the planted value:\n%s", out.String())
	}
	if fallbackOut.Len() != 0 {
		t.Fatalf("the process default logger got:\n%s", fallbackOut.String())
	}
}

package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func call(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, func(string) string { return "" }, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersionOutputCarriesRunnerPrefixAndVersion(t *testing.T) {
	code, out, errOut := call("--version")
	if code != 0 || !strings.HasPrefix(out, "cockpit-runner ") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
}

func TestNoArgumentsOrUnknownCommandPrintsUsageAndExitsTwo(t *testing.T) {
	for _, args := range [][]string{nil, {"fly"}} {
		code, _, errOut := call(args...)
		if code != 2 || !strings.Contains(errOut, "usage") {
			t.Fatalf("%v: exit %d stderr %q", args, code, errOut)
		}
	}
}

func TestSubcommandArgumentErrorsExitTwoWithReason(t *testing.T) {
	cases := [][]string{{"run"}, {"run", "Bad/name"}, {"probe"}, {"probe", "x"}, {"probe", "70000"}, {"edge", "nope"}}
	for _, args := range cases {
		code, _, errOut := call(args...)
		if code != 2 || errOut == "" {
			t.Fatalf("%v: exit %d stderr %q", args, code, errOut)
		}
	}
}

func TestEdgeReturnsZeroWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	var out, errOut bytes.Buffer
	if code := run(ctx, []string{"edge", fmt.Sprintf("%d=127.0.0.1:1", port)}, func(string) string { return "" }, &out, &errOut); code != 0 {
		t.Fatalf("exit %d stderr %q", code, errOut.String())
	}
}

func TestAnEdgeWithNoRoutesWaitsForTheContextAndReturnsZero(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	var out, errOut bytes.Buffer
	go func() { done <- run(ctx, []string{"edge"}, func(string) string { return "" }, &out, &errOut) }()
	select {
	case code := <-done:
		t.Fatalf("an edge with no routes returned %d before its context ended: %q", code, errOut.String())
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	if code := <-done; code != 0 || errOut.Len() != 0 {
		t.Fatalf("exit %d stderr %q", code, errOut.String())
	}
}

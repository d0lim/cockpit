package main

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/runner/otlp"
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
	cases := [][]string{{"run"}, {"run", "Bad/name"}, {"probe"}, {"probe", "x"}, {"probe", "70000"}, {"edge", "nope"}, {"otlp"}, {"otlp", "4318"}, {"otlp", ":70000"}, {"otlp", ":4318", ":4319"}}
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

func TestOTLPReturnsZeroWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	var out, errOut bytes.Buffer
	go func() {
		done <- run(ctx, []string{"otlp", "127.0.0.1:0"}, func(string) string { return "" }, &out, &errOut)
	}()
	select {
	case code := <-done:
		t.Fatalf("the receiver returned %d before its context ended: %q", code, errOut.String())
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	if code := <-done; code != 0 || out.Len() != 0 || !strings.Contains(errOut.String(), "otlp receiver listening") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out.String(), errOut.String())
	}
}

func TestOTLPExitsTwoWhenItsAddressIsTaken(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if code, _, errOut := call("otlp", ln.Addr().String()); code != 2 || errOut == "" {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}

func TestOTLPSetsASoftMemoryLimitUnlessTheEnvironmentSetsOne(t *testing.T) {
	before := debug.SetMemoryLimit(-1)
	t.Cleanup(func() { debug.SetMemoryLimit(before) })
	serve := func(env string) int64 {
		debug.SetMemoryLimit(math.MaxInt64)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var out, errOut bytes.Buffer
		if code := run(ctx, []string{"otlp", "127.0.0.1:0"}, func(k string) string {
			if k == "GOMEMLIMIT" {
				return env
			}
			return ""
		}, &out, &errOut); code != 0 {
			t.Fatalf("exit %d stderr %q", code, errOut.String())
		}
		return debug.SetMemoryLimit(-1)
	}
	if got := serve(""); got != otlp.MemoryLimit {
		t.Fatalf("soft memory limit %d without GOMEMLIMIT, want %d", got, otlp.MemoryLimit)
	}
	if got := serve("1GiB"); got != math.MaxInt64 {
		t.Fatalf("soft memory limit %d with GOMEMLIMIT set, want the runtime's own value left alone", got)
	}
}

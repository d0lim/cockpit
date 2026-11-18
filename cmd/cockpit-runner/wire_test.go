package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"net"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/mask"
	recorder "github.com/d0lim/cockpit/internal/runner/wire"
	"github.com/d0lim/cockpit/internal/wire"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func routeArg(t *testing.T) string {
	return strconv.Itoa(freePort(t)) + "=127.0.0.1:1"
}

func noEnv(string) string { return "" }

func TestWireAndWireCtlRefuseBadArgumentsWithExitTwoAndAReason(t *testing.T) {
	route := routeArg(t)
	cases := [][]string{
		{"wire"},
		{"wire", "--project"},
		{"wire", "--project", "r-1"},
		{"wire", route},
		{"wire", "--project", "r-1", "nope"},
		{"wire", "--project", "r-1", "--public-field"},
		{"wire", "--project", "r-1", "--public-field", "token", route},
		{"wire", "--project", "r-1", "--public-field", "token=%%%", route},
		{"wire", "--project", "r-1", "--public-field", "token=" + base64.RawURLEncoding.EncodeToString([]byte(".*")), route},
		{"wire", "--project", "r-1", "--public-field", "password=" + base64.RawURLEncoding.EncodeToString([]byte("p-[0-9]{8}")), route},
		{"wire-ctl"},
		{"wire-ctl", "ping", "extra"},
		{"wire-ctl", "arm"},
		{"wire-ctl", "take", "bad name"},
		{"wire-ctl", "sing", "x"},
	}
	for _, args := range cases {
		code, _, errOut := call(args...)
		if code != 2 || errOut == "" {
			t.Errorf("%v: exit %d stderr %q", args, code, errOut)
		}
	}
}

func TestWireReturnsZeroWhenItsContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	var errOut bytes.Buffer
	field := "token=" + base64.RawURLEncoding.EncodeToString([]byte("tok-[0-9a-f]{8}"))
	go func() {
		done <- serveWire(ctx, []string{"--project", "r-1", "--public-field", field, routeArg(t)}, noEnv, &errOut, 0)
	}()
	select {
	case code := <-done:
		t.Fatalf("the recorder returned %d before its context ended: %q", code, errOut.String())
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	if code := <-done; code != 0 || !strings.Contains(errOut.String(), "wire recorder listening") {
		t.Fatalf("exit %d stderr %q", code, errOut.String())
	}
}

func TestWireExitsTwoWhenARouteIsAlreadyTaken(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var errOut bytes.Buffer
	args := []string{"--project", "r-1", strconv.Itoa(ln.Addr().(*net.TCPAddr).Port) + "=127.0.0.1:1"}
	if code := serveWire(context.Background(), args, noEnv, &errOut, 0); code != 2 || errOut.Len() == 0 {
		t.Fatalf("exit %d stderr %q", code, errOut.String())
	}
}

func TestWireSetsASoftMemoryLimitUnlessTheEnvironmentSetsOne(t *testing.T) {
	before := debug.SetMemoryLimit(-1)
	t.Cleanup(func() { debug.SetMemoryLimit(before) })
	serve := func(env string) int64 {
		debug.SetMemoryLimit(math.MaxInt64)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var errOut bytes.Buffer
		getenv := func(k string) string {
			if k == "GOMEMLIMIT" {
				return env
			}
			return ""
		}
		if code := serveWire(ctx, []string{"--project", "r-1", routeArg(t)}, getenv, &errOut, 0); code != 0 {
			t.Fatalf("exit %d stderr %q", code, errOut.String())
		}
		return debug.SetMemoryLimit(-1)
	}
	if got := serve(""); got != recorder.MemoryLimit {
		t.Fatalf("soft memory limit %d, want %d", got, recorder.MemoryLimit)
	}
	if got := serve("1GiB"); got != math.MaxInt64 {
		t.Fatalf("soft memory limit %d with GOMEMLIMIT set", got)
	}
}

func startRecorder(t *testing.T) string {
	t.Helper()
	masker, err := mask.New(mask.Options{Mode: mask.Runner})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := recorder.Listen(recorder.Options{Project: "r-1", Routes: []recorder.Route{{Listen: freePort(t), Service: "127.0.0.1", Port: 1}}, Masker: masker})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- rec.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return rec.ControlAddr()
}

func TestWireCtlPingArmAndTakeSpeakToTheRecorderAndATakeEndsWithItsEndLine(t *testing.T) {
	addr := startRecorder(t)
	ctl := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := wireCtl(context.Background(), addr, args, &out, &errOut)
		return code, out.String(), errOut.String()
	}
	if code, out, errOut := ctl("ping"); code != 0 || out != "" || errOut != "" {
		t.Fatalf("ping: exit %d out %q err %q", code, out, errOut)
	}
	if code, _, errOut := ctl("arm", "exec-1"); code != 0 || errOut != "" {
		t.Fatalf("arm: exit %d err %q", code, errOut)
	}
	code, out, errOut := ctl("take", "exec-1")
	var line wire.Line
	if code != 0 || errOut != "" || json.Unmarshal([]byte(out), &line) != nil || line.End == nil {
		t.Fatalf("take: exit %d out %q err %q", code, out, errOut)
	}
	code, out, errOut = ctl("take", "other")
	if code == 0 || out != "" || !strings.Contains(errOut, `"other"`) {
		t.Fatalf("take of another name: exit %d out %q err %q", code, out, errOut)
	}
}

func TestWireCtlExitsOneWithAReasonWhenNoRecorderListens(t *testing.T) {
	var out, errOut bytes.Buffer
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(freePort(t)))
	if code := wireCtl(context.Background(), addr, []string{"ping"}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "not reachable") {
		t.Fatalf("exit %d stderr %q", code, errOut.String())
	}
}

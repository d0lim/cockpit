package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const id = "run-20261019-010000-abc1234"

type canned struct {
	mu      sync.Mutex
	status  int
	body    any
	byPath  map[string]any
	headers []http.Header
}

func (c *canned) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	c.headers = append(c.headers, r.Header.Clone())
	status, body := c.status, c.body
	if b, ok := c.byPath[r.URL.Path]; ok {
		status, body = http.StatusOK, b
	}
	if status == 0 {
		status, body = http.StatusNotFound, map[string]any{"code": "NOT_FOUND", "message": "not found"}
	}
	c.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

type run struct {
	code   int
	stdout string
	stderr string
}

func invoke(t *testing.T, url string, argv ...string) run {
	t.Helper()
	var out, errOut bytes.Buffer
	env := map[string]string{EnvURL: url}
	io := IO{
		Getenv: func(k string) string { return env[k] }, Cwd: "/w", Stdout: &out, Stderr: &errOut,
		Poll: 5 * time.Millisecond, Reconnect: 50 * time.Millisecond,
		Toplevel: func(context.Context, string) (string, error) { return "/w", nil },
	}
	code := Main(context.Background(), append([]string{"runs"}, argv...), io)
	return run{code: code, stdout: out.String(), stderr: errOut.String()}
}

func hub(t *testing.T, c *canned) string {
	t.Helper()
	srv := httptest.NewServer(c)
	t.Cleanup(srv.Close)
	return srv.URL
}

func deadURL(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return "http://" + addr
}

func item(phase string, verdict any, extra map[string]any) map[string]any {
	m := map[string]any{"id": id, "phase": phase, "env": "up", "verdict": verdict, "sha": "abc", "suite": "toy", "hostApi": 1, "cockpitVersion": "0.1.0",
		"submitter": "cli", "keep": false, "createdAt": "2026-10-19T01:00:00.000Z", "phaseLabel": phase, "verdictLabel": verdict}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestHubRefusalsMapToExitCodesConflictIsFiveAndOtherClientErrorsAreThree(t *testing.T) {
	cases := []struct {
		status int
		code   string
		exit   int
	}{
		{409, "CONFLICT_STATE", ExitConflict},
		{409, "CONFLICT_OWNER", ExitConflict},
		{400, "BAD_REQUEST", ExitRejected},
		{404, "NOT_FOUND", ExitRejected},
		{403, "", ExitRejected},
		{500, "INTERNAL", ExitError},
	}
	for _, c := range cases {
		body := map[string]any{"message": "no"}
		if c.code != "" {
			body["code"] = c.code
		}
		url := hub(t, &canned{status: c.status, body: body})
		got := invoke(t, url, "stop", id, "--json")
		if got.code != c.exit {
			t.Errorf("%d %s: exit %d, want %d (%s)", c.status, c.code, got.code, c.exit, got.stdout)
		}
	}
	if len(cases) != 6 {
		t.Fatal("case count")
	}
}

func TestANonRespondingHubIsFourAndNamesTheCommandThatStartsIt(t *testing.T) {
	url := deadURL(t)
	got := invoke(t, url, "list")
	if got.code != ExitUnreachable || !strings.Contains(got.stderr, "cockpit hub") || !strings.Contains(got.stderr, "COCKPIT_PORT=") {
		t.Fatalf("%+v", got)
	}
	if got := invoke(t, url, "wait", id); got.code != ExitUnreachable {
		t.Fatalf("wait on a dead hub: %+v", got)
	}
}

func TestWaitPastTheTimeoutIsSixAndThePoolAllKeptQueueIsSeven(t *testing.T) {
	queued := hub(t, &canned{byPath: map[string]any{"/api/runs/" + id: item("queued", nil, map[string]any{"position": 2, "waitReason": "active-run"})}})
	if got := invoke(t, queued, "wait", id, "--timeout", "0.05"); got.code != ExitTimeout || !strings.Contains(got.stdout, "timed out") {
		t.Fatalf("timeout: %+v", got)
	}
	submitted := hub(t, &canned{status: 202, body: map[string]any{"id": id, "phase": "queued", "position": 2, "merged": false}, byPath: map[string]any{"/api/runs/" + id: item("queued", nil, map[string]any{"position": 2, "waitReason": "active-run"})}})
	if got := invoke(t, submitted, "submit", "--wait", "--timeout", "0.05", "--json"); got.code != ExitTimeout || !strings.Contains(got.stdout, `"code":"TIMEOUT"`) || !strings.Contains(got.stdout, id) {
		t.Fatalf("submit --wait timeout: %+v", got)
	}
	kept := hub(t, &canned{byPath: map[string]any{"/api/runs/" + id: item("queued", nil, map[string]any{"position": 1, "waitReason": "pool-all-kept"})}})
	if got := invoke(t, kept, "wait", id, "--json"); got.code != ExitHuman || !strings.Contains(got.stdout, `"code":"WAITING_ON_HUMAN"`) {
		t.Fatalf("pool-all-kept: %+v", got)
	}
}

func TestAFinishedRunEndsTheWaitWithItsVerdictExitCode(t *testing.T) {
	want := map[string]int{"passed": ExitOK, "failed": ExitFailed, "error": ExitError, "aborted": ExitConflict}
	for verdict, exit := range want {
		url := hub(t, &canned{byPath: map[string]any{"/api/runs/" + id: item("done", verdict, nil)}})
		if got := invoke(t, url, "wait", id); got.code != exit || !strings.Contains(got.stdout, "report: "+Cmd+" report "+id) {
			t.Errorf("%s: %+v", verdict, got)
		}
	}
	lost := hub(t, &canned{byPath: map[string]any{"/api/runs/" + id: item("done", nil, map[string]any{"mark": "lost"})}})
	if got := invoke(t, lost, "wait", id); got.code != ExitError {
		t.Fatalf("lost run: %+v", got)
	}
}

func TestEveryJSONLineIsOneObjectWithAStableCode(t *testing.T) {
	stable := StableCodes()
	urls := map[string]string{
		"dead":     deadURL(t),
		"conflict": hub(t, &canned{status: 409, body: map[string]any{"code": "CONFLICT_CAP", "message": "cap"}}),
		"list":     hub(t, &canned{byPath: map[string]any{"/api/runs": map[string]any{"runs": []any{item("done", "passed", nil)}, "orphans": []any{}, "pool": map[string]any{"size": 1, "max": 5}, "up": map[string]any{"count": 1, "cap": 3}, "dockerDown": false, "tickError": nil}}}),
		"submit":   hub(t, &canned{status: 202, body: map[string]any{"id": id, "phase": "queued", "position": 1, "merged": false}}),
	}
	commands := [][]string{{"list"}, {"stop", id}, {"wait", id}, {"submit"}, {"bogus"}, {"get", "not-a-run"}, {"exec", id}}
	lines := 0
	for name, url := range urls {
		for _, cmd := range commands {
			got := invoke(t, url, append(cmd, "--json")...)
			out := strings.TrimSuffix(got.stdout, "\n")
			var doc map[string]any
			if strings.Count(got.stdout, "\n") != 1 || json.Unmarshal([]byte(out), &doc) != nil {
				t.Errorf("%s %v: not one JSON line: %q", name, cmd, got.stdout)
				continue
			}
			if code, _ := doc["code"].(string); !slices.Contains(stable, code) || doc["schemaVersion"] != float64(1) {
				t.Errorf("%s %v: code %v", name, cmd, doc["code"])
			}
			lines++
		}
	}
	if lines != len(urls)*len(commands) {
		t.Fatalf("checked %d lines", lines)
	}
}

func TestUsageErrorsAreThreeAndWritesCarryTheGuardHeaders(t *testing.T) {
	c := &canned{status: 200, body: map[string]any{"run": item("done", "passed", map[string]any{"keep": true})}}
	url := hub(t, c)
	for _, argv := range [][]string{{}, {"bogus"}, {"stop"}, {"stop", "nope"}, {"list", "extra"}, {"wait", id, "--timeout", "0"}, {"submit", "--suite"}, {"exec", id}} {
		if got := invoke(t, url, argv...); got.code != ExitRejected {
			t.Errorf("%v: %+v", argv, got)
		}
	}
	if got := invoke(t, url, "keep", id, "--force"); got.code != ExitOK || !strings.Contains(got.stdout, "keep") {
		t.Fatalf("keep: %+v", got)
	}
	last := c.headers[len(c.headers)-1]
	if last.Get("X-Cockpit") != "1" || last.Get("Origin") != url {
		t.Fatalf("headers %v", last)
	}
}

func TestHelpPrintsTheUsageAndExitsZeroWithoutCallingTheHub(t *testing.T) {
	url := deadURL(t)
	for _, argv := range [][]string{{"--help"}, {"-h"}, {"help"}, {"submit", "--help"}, {"stop", id, "-h"}} {
		got := invoke(t, url, argv...)
		if got.code != ExitOK || !strings.Contains(got.stdout, "usage: ") || !strings.Contains(got.stdout, "submit [--commit") {
			t.Errorf("%v: %+v", argv, got)
		}
	}
	if got := invoke(t, url, "exec", id, "--", "-h"); got.code == ExitOK {
		t.Fatalf("a -h after -- is a flow argument, not help: %+v", got)
	}
}

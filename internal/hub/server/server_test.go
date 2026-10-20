package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/hub/cli"
	"github.com/d0lim/cockpit/internal/hub/hubtest"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/registry"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/store"
)

type running struct {
	url   string
	addr  string
	world *hubtest.World
	stop  func() error
	home  string
	env   map[string]string
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func start(t *testing.T, home string, verdict lifecycle.Verdict) *running {
	t.Helper()
	return startIn(t, filepath.Join(t.TempDir(), "registry"), home, verdict)
}

func startIn(t *testing.T, registryDir, home string, verdict lifecycle.Verdict) *running {
	t.Helper()
	r := &running{home: home, env: map[string]string{"COCKPIT_HOME": home, "COCKPIT_PORT": strconv.Itoa(freePort(t)), registry.EnvRegistry: registryDir}}
	ready := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			Getenv: func(k string) string { return r.env[k] },
			Build: func(in BuildInput) (Built, error) {
				r.world = hubtest.New(in.Store)
				r.world.Verdict = verdict
				return Built{Scheduler: scheduler.New(r.world.Deps())}, nil
			},
			Ready: func(addr string) { ready <- addr },
			Sync:  20 * time.Millisecond,
		})
	}()
	select {
	case r.addr = <-ready:
	case err := <-done:
		cancel()
		t.Fatalf("the hub did not start: %v", err)
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("the hub did not start in 5s")
	}
	r.url = "http://" + r.addr
	r.stop = sync.OnceValue(func() error { cancel(); return <-done })
	t.Cleanup(func() { r.stop() })
	return r
}

func runCLI(t *testing.T, url string, argv ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := cli.Main(context.Background(), append([]string{"runs"}, argv...), cli.IO{
		Getenv: func(k string) string { return map[string]string{cli.EnvURL: url}[k] }, Cwd: "/w", Stdout: &out, Stderr: &errOut,
		Poll: 10 * time.Millisecond, Toplevel: func(context.Context, string) (string, error) { return "/toy", nil },
	})
	return code, out.String() + errOut.String()
}

func TestTheHubListensOnLoopbackOnlyAtThePortItWasGiven(t *testing.T) {
	r := start(t, filepath.Join(t.TempDir(), "home"), lifecycle.VerdictPassed)
	host, port, err := net.SplitHostPort(r.addr)
	if err != nil || !net.ParseIP(host).IsLoopback() || port != r.env["COCKPIT_PORT"] {
		t.Fatalf("listening on %s", r.addr)
	}
	if _, err := os.Stat(filepath.Join(r.home, "hub.lock")); err != nil {
		t.Fatalf("no hub.lock: %v", err)
	}
	if err := r.stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.home, "hub.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("hub.lock left after stop: %v", err)
	}
}

func TestSubmitWithWaitOnTheToyConsumerEndsWithTheVerdictExitCode(t *testing.T) {
	passed := start(t, filepath.Join(t.TempDir(), "home"), lifecycle.VerdictPassed)
	code, out := runCLI(t, passed.url, "submit", "--suite", "toy", "--wait", "--timeout", "10")
	if code != cli.ExitOK || !strings.Contains(out, "verdict Passed") {
		t.Fatalf("passed: exit %d\n%s", code, out)
	}
	failed := start(t, filepath.Join(t.TempDir(), "home"), lifecycle.VerdictFailed)
	code, out = runCLI(t, failed.url, "submit", "--wait", "--json")
	if code != cli.ExitFailed || !strings.Contains(out, `"code":"FAILED"`) {
		t.Fatalf("failed: exit %d\n%s", code, out)
	}
	code, out = runCLI(t, passed.url, "list")
	if code != cli.ExitOK || !strings.Contains(out, "Done (up)") || !strings.Contains(out, "kept 1/5") {
		t.Fatalf("list: exit %d\n%s", code, out)
	}
}

func TestTwoHubsOnOneRegistryListEachOtherWithThisHubFirstAndTheOtherUpAtItsAddress(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "registry")
	first := startIn(t, dir, filepath.Join(t.TempDir(), "first"), lifecycle.VerdictPassed)
	second := startIn(t, dir, filepath.Join(t.TempDir(), "second"), lifecycle.VerdictPassed)
	type hub struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Self bool   `json:"self"`
		Up   bool   `json:"up"`
		URL  string `json:"url"`
	}
	read := func(r *running) []hub {
		res, err := http.Get(r.url + "/api/runs")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var list struct {
			Hubs []hub `json:"hubs"`
		}
		if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
			t.Fatal(err)
		}
		return list.Hubs
	}
	got := read(first)
	want := []hub{{ID: registry.ID(first.home), Name: "first", Self: true, Up: true, URL: first.url}, {ID: registry.ID(second.home), Name: "second", Up: true, URL: second.url}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("first sees %+v, want %+v", got, want)
	}
	if err := second.stop(); err != nil {
		t.Fatal(err)
	}
	got = read(first)
	if len(got) != 2 || got[1].Up || got[1].URL != "" || got[1].Name != "second" {
		t.Fatalf("after the second stopped: %+v", got)
	}
}

func TestTheRunsEventFollowsASubmitOnTheHubStream(t *testing.T) {
	r := start(t, filepath.Join(t.TempDir(), "home"), lifecycle.VerdictPassed)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, r.url+"/api/events?target=hub", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if code, out := runCLI(t, r.url, "submit"); code != cli.ExitOK {
		t.Fatalf("submit: %d %s", code, out)
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var ev struct {
			Kind string `json:"kind"`
			List struct {
				Runs []struct {
					ID string `json:"id"`
				} `json:"runs"`
			} `json:"list"`
		}
		if json.Unmarshal([]byte(data), &ev) == nil && ev.Kind == "runs" && len(ev.List.Runs) == 1 && store.IsRunID(ev.List.Runs[0].ID) {
			return
		}
	}
	t.Fatal("no runs event with the submitted run")
}

func TestAHomeWithThePreviousLayoutIsRefusedBeforeListening(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	os.MkdirAll(home, 0o700)
	os.WriteFile(filepath.Join(home, "state.json"), []byte(`{"schemaVersion":1,"queue":[],"leases":{}}`), 0o600)
	port := freePort(t)
	env := map[string]string{"COCKPIT_HOME": home, "COCKPIT_PORT": strconv.Itoa(port)}
	built := false
	err := Run(context.Background(), Options{Getenv: func(k string) string { return env[k] }, Build: func(BuildInput) (Built, error) { built = true; return Built{}, nil }})
	if !errors.Is(err, store.ErrLegacyLayout) || built {
		t.Fatalf("err %v, built %v", err, built)
	}
	if conn, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port)); err == nil {
		conn.Close()
		t.Fatal("something listens on the port")
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
)

const hostIndependentRunningCap = 1000

type e2e struct {
	t    *testing.T
	bin  string
	home string
	repo string
	url  string
	env  []string
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_COUNT=4", "GIT_CONFIG_KEY_0=maintenance.auto", "GIT_CONFIG_VALUE_0=false", "GIT_CONFIG_KEY_1=gc.auto", "GIT_CONFIG_VALUE_1=0", "GIT_CONFIG_KEY_2=maintenance.autoDetach", "GIT_CONFIG_VALUE_2=false", "GIT_CONFIG_KEY_3=gc.autoDetach", "GIT_CONFIG_VALUE_3=false",
		"GIT_AUTHOR_NAME=toy", "GIT_AUTHOR_EMAIL=toy@example.invalid", "GIT_COMMITTER_NAME=toy", "GIT_COMMITTER_EMAIL=toy@example.invalid")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func toyConsumer(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "toy")
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "testdata", "toy-project"))); err != nil {
		t.Fatal(err)
	}
	for _, generated := range []string{".env", compose.PrivateEnvName, "mock/bin"} {
		os.RemoveAll(filepath.Join(dir, generated))
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-q", "-m", "toy"}} {
		gitRun(t, dir, args...)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func (e *e2e) cockpit(dir string, args ...string) (int, string, string) {
	e.t.Helper()
	cmd := exec.Command(e.bin, args...)
	cmd.Dir, cmd.Env = dir, e.env
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		e.t.Fatal(err)
	}
	return code, out.String(), errOut.String()
}

func (e *e2e) startHub() {
	e.t.Helper()
	e.t.Cleanup(e.downLeftovers)
	var logs bytes.Buffer
	hub := exec.Command(e.bin, "hub")
	hub.Env, hub.Stderr, hub.Stdout = e.env, &logs, &logs
	if err := hub.Start(); err != nil {
		e.t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if code, _, _ := e.cockpit(e.repo, "runs", "list"); code == 0 {
			break
		}
		if time.Now().After(deadline) {
			hub.Process.Kill()
			e.t.Fatalf("the hub never answered:\n%s", logs.String())
		}
		time.Sleep(200 * time.Millisecond)
	}
	e.t.Cleanup(func() {
		hub.Process.Signal(syscall.SIGINT)
		done := make(chan error, 1)
		go func() { done <- hub.Wait() }()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			hub.Process.Kill()
		}
		if strings.Contains(logs.String(), "level=ERROR") {
			e.t.Errorf("hub log has errors:\n%s", logs.String())
		}
		e.t.Logf("hub log lines: %d", strings.Count(logs.String(), "\n"))
	})
	e.t.Cleanup(e.destroyRuns)
}

func (e *e2e) runIDs() []string {
	entries, err := os.ReadDir(filepath.Join(e.home, "runs"))
	if err != nil {
		return nil
	}
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			ids = append(ids, entry.Name())
		}
	}
	return ids
}

func (e *e2e) destroyRuns() {
	for _, id := range e.runIDs() {
		if code, out, errOut := e.cockpit(e.repo, "runs", "destroy", id, "--force"); code != 0 {
			e.t.Logf("runs destroy %s: exit %d\n%s%s", id, code, out, errOut)
		}
	}
}

func (e *e2e) downLeftovers() {
	for _, id := range e.runIDs() {
		out, err := exec.Command("docker", "ps", "-aq", "--filter", "label=com.docker.compose.project="+id).Output()
		if err != nil || len(bytes.Fields(out)) == 0 {
			continue
		}
		e.t.Errorf("run %s left %d containers after the hub stopped; bringing the project down", id, len(bytes.Fields(out)))
		if down, err := exec.Command("docker", "compose", "-p", id, "down", "-v", "--remove-orphans").CombinedOutput(); err != nil {
			e.t.Errorf("compose down %s: %v\n%s", id, err, down)
		}
	}
}

type collectorRead struct {
	Signals []struct {
		Seq       int64  `json:"seq"`
		Collector string `json:"collector"`
	} `json:"signals"`
	Collectors []struct {
		ID       string `json:"id"`
		Failures int    `json:"failures"`
		Reason   string `json:"reason"`
	} `json:"collectors"`
}

func (e *e2e) signals(id string) collectorRead {
	e.t.Helper()
	res, err := http.Get(e.url + "/api/runs/" + id + "/signals?limit=5000")
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var r collectorRead
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil || res.StatusCode != http.StatusOK {
		e.t.Fatalf("signals read status %d: %v", res.StatusCode, err)
	}
	return r
}

func (r collectorRead) count(collector string) int {
	n := 0
	for _, s := range r.Signals {
		if s.Collector == collector {
			n++
		}
	}
	return n
}

func newE2E(t *testing.T) *e2e {
	t.Helper()
	e := &e2e{t: t, home: filepath.Join(t.TempDir(), "home"), repo: toyConsumer(t), bin: filepath.Join(t.TempDir(), "cockpit")}
	if out, err := exec.Command("go", "build", "-o", e.bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	if err := os.MkdirAll(e.home, 0o700); err != nil {
		t.Fatal(err)
	}
	block := strconv.Itoa(30000 + rand.IntN(150)*100)
	config := "allowedRepositories:\n  - " + e.repo + "\nlimits:\n  runningCap: " + strconv.Itoa(hostIndependentRunningCap) + "\nports:\n  base: " + block + "\n  slots: 1\n"
	if err := os.WriteFile(filepath.Join(e.home, "config.yaml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(freePort(t))
	e.url = "http://127.0.0.1:" + port
	e.env = append(os.Environ(), "COCKPIT_HOME="+e.home, "COCKPIT_PORT="+port, "COCKPIT_URL="+e.url, "COCKPIT_STOP_ON_DONE=0")
	return e
}

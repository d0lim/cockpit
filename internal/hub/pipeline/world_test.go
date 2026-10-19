package pipeline

import (
	"context"
	"math/rand/v2"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/config"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/runnerbin"
	"github.com/d0lim/cockpit/internal/version"
)

const planted = "PLANTED-secret-7f3a9c"

type world struct {
	t       *testing.T
	fake    *fakedocker.Fake
	sched   *scheduler.Scheduler
	hub     *Hub
	repo    string
	base    int
	mu      sync.Mutex
	events  []string
	ensures int
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=toy", "GIT_AUTHOR_EMAIL=toy@example.invalid", "GIT_COMMITTER_NAME=toy", "GIT_COMMITTER_EMAIL=toy@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func toyRepo(t *testing.T, mutate func(dir string)) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "toy")
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "..", "testdata", "toy-project"))); err != nil {
		t.Fatal(err)
	}
	for _, generated := range []string{".env", compose.PrivateEnvName, "mock/bin"} {
		os.RemoveAll(filepath.Join(dir, generated))
	}
	if mutate != nil {
		mutate(dir)
	}
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "toy")
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func passingSteps() []contract.ResultStep {
	return []contract.ResultStep{
		{Flow: "seed", ID: "seed-users", Status: contract.StepStatusPassed, Requests: 1},
		{Flow: "health", ID: "health-ok", Status: contract.StepStatusPassed, Requests: 1},
		{Flow: "orders", ID: "create", Status: contract.StepStatusPassed, Requests: 2},
		{Flow: "facts", ID: "facts-read", Status: contract.StepStatusPassed, Requests: 1},
	}
}

func toyFlows() []contract.ManifestFlow {
	return []contract.ManifestFlow{
		{ID: "seed", Stage: "basic", Once: true, Requires: []string{}},
		{ID: "health", Stage: "basic", Requires: []string{}},
		{ID: "orders", Stage: "basic", Requires: []string{"health", "seed"}},
		{ID: "facts", Stage: "observe", Requires: []string{"orders"}},
	}
}

func newWorld(t *testing.T, mutate func(*fakedocker.Fake), mutateRepo func(string)) *world {
	return newWorldAs(t, "", mutate, mutateRepo)
}

func newWorldAs(t *testing.T, runnerUser string, mutate func(*fakedocker.Fake), mutateRepo func(string)) *world {
	return newWorldWith(t, func(o *Options) { o.RunnerUser = runnerUser }, mutate, mutateRepo)
}

func newWorldWith(t *testing.T, option func(*Options), mutate func(*fakedocker.Fake), mutateRepo func(string)) *world {
	t.Helper()
	w := &world{t: t, repo: toyRepo(t, mutateRepo), base: freeBlock(t)}
	w.fake = &fakedocker.Fake{
		ConfigFixture: filepath.Join("..", "..", "seal", "testdata", "config.toy.json"),
		CanaryPort:    w.base + compose.CanaryOffset,
		Version:       version.String(),
		RunnerLogs:    "health › health-ok passed\ntoken " + planted + " leaked\n",
		EnvFiles: map[string]string{
			".env":                 "POSTGRES_USER=toy\nPOSTGRES_DB=toy\nPOSTGRES_PASSWORD=dbpass-0123456789abcdef\nAPP_TOKEN=toy_abcdefghijklmnopqrstuvwx\n",
			compose.PrivateEnvName: `{"local":{"appToken":"` + planted + `"}}`,
		},
		Steps: passingSteps(),
		Flows: toyFlows(),
	}
	if mutate != nil {
		mutate(w.fake)
	}
	stageLimits := stages.Limits{Build: time.Minute, Boot: 2 * time.Second, Suite: 300 * time.Millisecond, Call: time.Minute, Poll: 10 * time.Millisecond}
	limits := config.Defaults()
	limits.PortBase = w.base
	cfg := config.Config{AllowedRepositories: []string{w.repo}, Limits: limits, ReservedPorts: []config.PortRange{}}
	opts := Options{
		Home: t.TempDir(), Config: cfg, HubPort: 7710, DockerFor: w.fake.For, Probe: fakedocker.Reconcile{},
		Ensure: func(ctx context.Context) (runnerbin.Image, error) {
			w.mu.Lock()
			w.ensures++
			w.mu.Unlock()
			return runnerbin.Image{Tag: fakedocker.RunnerTag, ID: fakedocker.PinnedID, Arch: "arm64"}, nil
		},
		Limits: stageLimits, NoCollection: true, PortProbe: func(int) bool { return true }, TickInterval: 20 * time.Millisecond,
		Events: func(e Event) { w.mu.Lock(); w.events = append(w.events, e.Line); w.mu.Unlock() },
	}
	option(&opts)
	sched, hub, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sched.Dispose(); hub.Close() })
	w.sched, w.hub = sched, hub
	return w
}

func (w *world) submit() store.Meta {
	w.t.Helper()
	res := w.sched.Submit(context.Background(), scheduler.SubmitInput{Worktree: w.repo, Submitter: "tester"})
	if res.Fail != nil {
		w.t.Fatalf("submit: %+v", res.Fail)
	}
	w.sched.Tick()
	w.sched.Idle()
	return w.meta(res.ID)
}

func (w *world) meta(id string) store.Meta {
	w.t.Helper()
	m, err := w.hub.Store().MustRead(id)
	if err != nil {
		w.t.Fatal(err)
	}
	return m
}

func (w *world) verbs() []string {
	var out []string
	for _, c := range w.fake.Calls() {
		out = append(out, fakedocker.Verb(c.Args))
	}
	return out
}

func (w *world) eventText() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.events, "\n")
}

func indexOf(list []string, v string) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return -1
}

func replaceIn(t *testing.T, path, old, new string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), old) {
		t.Fatalf("%s lacks %q: %v", path, old, err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), old, new, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func freeBlock(t *testing.T) int {
	t.Helper()
	for range 50 {
		base := 10000 + rand.IntN(380)*100
		l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(base+compose.CanaryOffset)))
		if err == nil {
			l.Close()
			return base
		}
	}
	t.Fatal("no free port block")
	return 0
}

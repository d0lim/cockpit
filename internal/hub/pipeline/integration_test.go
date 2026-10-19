package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/config"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/procgroup"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/runnerbin"
)

func TestIntegrationToySubmissionPassesThroughTheHubPipelineThenExecsStopsResumesAndDestroys(t *testing.T) {
	if os.Getenv("COCKPIT_INTEGRATION") != "1" {
		t.Skip("needs COCKPIT_INTEGRATION=1, a Docker daemon and scripts/build-runners.sh")
	}
	home := t.TempDir()
	pins := runnerbin.FilePins{Path: filepath.Join(home, "runner-images.json")}
	ensure := func(ctx context.Context) (runnerbin.Image, error) {
		return runnerbin.Ensure(ctx, runnerbin.CLI{}, runnerbin.Lookup, pins)
	}
	if _, err := ensure(t.Context()); errors.Is(err, runnerbin.ErrNotEmbedded) {
		t.Skip("the runner is not embedded; run scripts/build-runners.sh first")
	}
	repo := toyRepo(t, func(dir string) {
		replaceIn(t, filepath.Join(dir, "scripts", "flows", "health.ts"), "if (!value) throw", "console.log(\"token \" + String(ctx.priv.appToken));\n\t\t\t\tif (!value) throw")
	})
	cfg := config.Config{AllowedRepositories: []string{repo}, Limits: config.Defaults(), ReservedPorts: []config.PortRange{}}
	sched, hub, err := New(Options{
		Home: home, Config: cfg, HubPort: 7710, Ensure: ensure, Limits: stages.DefaultLimits(), StopOnDone: false,
		DockerFor: func(h procgroup.Hooks) compose.Docker { return compose.CLI{Hooks: h} },
		Probe:     reconcile.NewCLI(), TickInterval: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Dispose()
	res := sched.Submit(t.Context(), scheduler.SubmitInput{Worktree: repo, Submitter: "tester"})
	if res.Fail != nil {
		t.Fatal(res.Fail)
	}
	t.Cleanup(func() {
		p := compose.Project{Docker: compose.CLI{}, Name: res.ID, Mask: func(s string) string { return s }}
		p.Down(context.Background())
	})
	deadline := time.Now().Add(20 * time.Minute)
	for {
		sched.Tick()
		sched.Idle()
		m, _ := hub.Store().MustRead(res.ID)
		if m.Phase == lifecycle.Done || time.Now().After(deadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	m, _ := hub.Store().MustRead(res.ID)
	if m.Phase != lifecycle.Done || m.Verdict != lifecycle.VerdictPassed || m.Env != lifecycle.EnvUp {
		t.Fatalf("state %+v failed stage %s notice %q tail %q", m.State, m.FailedStage, m.Notice, m.Tail)
	}
	t.Logf("passed: %s pinned %s (%s)", m.ID, m.RunnerImage.Tag, m.RunnerImage.ID)
	dir, _ := hub.Store().RunDir(m.ID)
	token := envValue(t, filepath.Join(dir, "src", ".env"), "APP_TOKEN")
	text := assertBundle(t, dir)
	doc, summary := readSummary(t, dir)
	stages, _ := doc["stages"].(map[string]any)
	for _, name := range []string{"source", "runner.manifest.json", "runner.result.json", "signals"} {
		if st, _ := stages[name].(map[string]any); st["status"] != "ok" {
			t.Errorf("stage %s: %v", name, stages[name])
		}
	}
	if doc["verdict"] != "passed" {
		t.Fatalf("summary verdict %v reasons %v", doc["verdict"], doc["reasons"])
	}
	if strings.Contains(text, token) || !strings.Contains(summary, "token ****") {
		t.Fatalf("the bundle holds the app token or the runner line is missing:\n%s", summary)
	}
	actor := scheduler.Actor{By: "tester"}
	if r := sched.Exec(m.ID, "health", actor); r.Fail != nil {
		t.Fatalf("exec health: %+v", r.Fail)
	} else {
		t.Logf("exec health: %+v", r.Result)
	}
	if r := sched.Stop(m.ID, actor); r.Fail != nil || r.Run.Env != lifecycle.EnvStopped {
		t.Fatalf("stop: %+v", r)
	}
	if r := sched.Resume(m.ID, actor); r.Fail != nil || r.Run.Env != lifecycle.EnvUp {
		t.Fatalf("resume: %+v", r.Fail)
	}
	if r := sched.Destroy(m.ID, actor); r.Fail != nil || r.Run.Env != lifecycle.EnvDestroyed {
		t.Fatalf("destroy: %+v", r.Fail)
	}
}

func envValue(t *testing.T, path, name string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, name+"="); ok && len(v) > 8 {
			return v
		}
	}
	t.Fatalf("%s has no %s", path, name)
	return ""
}

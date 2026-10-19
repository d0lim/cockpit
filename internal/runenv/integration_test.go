package runenv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/runnerbin"
	"github.com/d0lim/cockpit/internal/seal"
)

func toyProject(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "toy")
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "testdata", "toy-project"))); err != nil {
		t.Fatal(err)
	}
	for _, generated := range []string{".env", compose.PrivateEnvName, "mock/bin"} {
		os.RemoveAll(filepath.Join(dir, generated))
	}
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=toy", "GIT_AUTHOR_EMAIL=toy@example.invalid", "GIT_COMMITTER_NAME=toy", "GIT_COMMITTER_EMAIL=toy@example.invalid")
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-q", "-m", "toy"}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

var keptRunID = regexp.MustCompile(`docker compose -p (run-env-[0-9a-z-]+) `)

func TestIntegrationToyRunEnvPassesTheSealAndTheUnsealCheckTurnsTheProbesRed(t *testing.T) {
	if os.Getenv("COCKPIT_INTEGRATION") != "1" {
		t.Skip("needs COCKPIT_INTEGRATION=1, a Docker daemon and scripts/build-runners.sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	home := t.TempDir()
	pins := runnerbin.FilePins{Path: filepath.Join(home, "runner-images.json")}
	ensure := func(ctx context.Context) (runnerbin.Image, error) {
		return runnerbin.Ensure(ctx, runnerbin.CLI{}, runnerbin.Lookup, pins)
	}
	if _, err := ensure(ctx); errors.Is(err, runnerbin.ErrNotEmbedded) {
		t.Skip("the runner is not embedded; run scripts/build-runners.sh first")
	}
	runs := t.TempDir()
	var out bytes.Buffer
	opts := Options{Project: toyProject(t), UnsealCheck: true, Keep: true}
	code := Run(ctx, opts, Deps{Docker: compose.CLI{}, EnsureImage: ensure, Home: home, Out: &out, TempDir: runs})
	text := out.String()
	m := keptRunID.FindStringSubmatch(text)
	if m != nil {
		t.Cleanup(func() {
			p := compose.Project{Docker: compose.CLI{}, Name: m[1], Mask: func(s string) string { return s }}
			p.Down(context.Background())
		})
	}
	if code != ExitPassed {
		t.Fatalf("exit %d\n%s", code, text)
	}
	for _, want := range []string{"seal static (", "24/24 passed", "the probes turn red", "5 steps: 5 passed"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q\n%s", want, text)
		}
	}
	files, _ := filepath.Glob(filepath.Join(runs, "cockpit-run-env-*", seal.FileName))
	if len(files) != 1 {
		t.Fatalf("seal.json files %v", files)
	}
	var report struct {
		Verdict string            `json:"verdict"`
		Static  []json.RawMessage `json:"static"`
		Probes  []json.RawMessage `json:"probes"`
	}
	data, _ := os.ReadFile(files[0])
	if err := json.Unmarshal(data, &report); err != nil || report.Verdict != "passed" || len(report.Static) != 24 || len(report.Probes) != 8 {
		t.Fatalf("seal.json %s: %v", data, err)
	}
}

package runenv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/compose/composetest"
	"github.com/d0lim/cockpit/internal/runnerbin"
)

func includedCacheToy(t *testing.T) string {
	t.Helper()
	dir := toyProject(t)
	top := filepath.Join(dir, "compose.yaml")
	data, err := os.ReadFile(top)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	start, end := strings.Index(text, "\n  cache:\n"), strings.Index(text, "\nvolumes:\n")
	if start < 0 || end < start {
		t.Fatalf("the toy compose.yaml has no cache block before volumes:\n%s", text)
	}
	if err := os.MkdirAll(filepath.Join(dir, "deps", "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deps", "cache", "compose.yaml"), []byte("services:\n"+text[start+1:end]), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(top, []byte("include:\n  - deps/cache/compose.yaml\n"+text[:start+1]+text[end+1:]), 0o644); err != nil {
		t.Fatal(err)
	}
	composetest.Git(t, dir, "add", "-A")
	composetest.Git(t, dir, "commit", "-q", "-m", "include the cache from deps/cache")
	composetest.Submodule(t, dir, "deps/cache", "cache")
	return dir
}

func TestIntegrationAToyWhoseCacheComesFromAnIncludeInASubmodulePassesTheSealBootsAndRunsTheSuite(t *testing.T) {
	if os.Getenv("COCKPIT_INTEGRATION") != "1" {
		t.Skip("needs COCKPIT_INTEGRATION=1, a Docker daemon and scripts/build-runners.sh")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("needs Docker Desktop: the seal probes reach the host canary through host.docker.internal, which Docker on " + runtime.GOOS + " does not provide (docs/sealing.md, Probes)")
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
	project := includedCacheToy(t)
	if top, _ := os.ReadFile(filepath.Join(project, "compose.yaml")); strings.Contains(string(top), "\n  cache:\n") {
		t.Fatal("canary: the cache service is still defined in the top compose.yaml")
	}
	runs := t.TempDir()
	var out bytes.Buffer
	code := Run(ctx, Options{Project: project, Keep: true}, Deps{Docker: compose.CLI{}, EnsureImage: ensure, Home: home, Out: &out, TempDir: runs})
	text := out.String()
	if m := keptRunID.FindStringSubmatch(text); m != nil {
		t.Cleanup(func() {
			p := compose.Project{Docker: compose.CLI{}, Name: m[1], Mask: func(s string) string { return s }}
			p.Down(context.Background())
		})
	}
	if code != ExitPassed {
		t.Fatalf("exit %d\n%s", code, text)
	}
	for _, want := range []string{"seal static (", "24/24 passed", "5 steps: 5 passed"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q\n%s", want, text)
		}
	}
	included, _ := filepath.Glob(filepath.Join(runs, "cockpit-run-env-*", "src", "deps", "cache", "compose.yaml"))
	if len(included) != 1 {
		t.Fatalf("extracted included files %v", included)
	}
	if data, _ := os.ReadFile(included[0]); !strings.Contains(string(data), `"6379:6379"`) {
		t.Fatalf("canary: the included cache publishes no host port for the harness to reset:\n%s", data)
	}
}

package headless

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/cli"
	"github.com/d0lim/cockpit/internal/hub/procgroup"
	hubstages "github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/runnerbin"
)

var runIDMutations = []porttest.Mutation{
	{"a headless run never takes the id of a project or hub run on the host", "pass nil as the taken ids to store.NewRunID", "red: the run gets the id a container already carries", "TestTheRunIDSkipsIDsThatContainersNetworksOrHubRunsHold"},
	{"a failed bundle copy leaves no partial directory", "drop the RemoveAll after a failed fill in copyBundle", "red: the half-copied --out directory stays", "TestAFailedBundleCopyRemovesTheDirectoryItCreated"},
	{"the stage tail passes the run redactor", "return r.lines unmasked from tail", "red: the planted value shows in the tail", "TestTheStageTailMasksAPlantedValue"},
}

func TestRunIDMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "runid_test.go", runIDMutations)
}

type labelDocker map[string]string

func (d labelDocker) Run(ctx context.Context, cmd compose.Command) (compose.Result, error) {
	return compose.Result{Stdout: []byte(d[cmd.Args[0]])}, nil
}

func TestTheRunIDSkipsIDsThatContainersNetworksOrHubRunsHold(t *testing.T) {
	now, sha := time.Date(2026, 11, 23, 2, 0, 0, 0, time.UTC), "0123456789abcdef"
	base := store.NewRunID(now, sha, nil)
	home := t.TempDir()
	taken, err := takenRunIDs(context.Background(), labelDocker{}, home)
	if err != nil || len(taken) != 0 || store.NewRunID(now, sha, taken) != base {
		t.Fatalf("canary: an empty host gives %v %v", taken, err)
	}
	if err := os.MkdirAll(filepath.Join(home, "runs", base+"-2"), 0o700); err != nil {
		t.Fatal(err)
	}
	d := labelDocker{"ps": base + "\nrun-20260101-000000-aaaaaaa\n", "network": base + "-3\n"}
	taken, err = takenRunIDs(context.Background(), d, home)
	if err != nil || len(taken) != 4 {
		t.Fatalf("taken %v %v", taken, err)
	}
	if got := store.NewRunID(now, sha, taken); got != base+"-4" {
		t.Fatalf("id %s, want %s-4", got, base)
	}
	runHome := t.TempDir()
	project := toyProject(t)
	fixed := func() time.Time { return now }
	head := strings.TrimSpace(gitOut(t, project, "rev-parse", "HEAD"))
	held := store.NewRunID(now, head, nil)
	if err := os.MkdirAll(filepath.Join(runHome, "runs", held), 0o700); err != nil {
		t.Fatal(err)
	}
	f := newFake(nil)
	var out bytes.Buffer
	code := Run(context.Background(), Options{Project: project}, Deps{
		Docker: f.For(procgroup.Hooks{}), EnsureImage: func(context.Context) (runnerbin.Image, error) { return fakeImage, nil },
		Home: runHome, Out: &out, Free: func(int) bool { return true }, TempDir: t.TempDir(), Submitter: "tester", ListenCanary: canaryFor(f), Now: fixed,
	})
	if code != cli.ExitOK || !strings.Contains(out.String(), "run "+held+"-2") {
		t.Fatalf("exit %d: the run did not skip the id a hub run holds (%s)\n%s", code, held, out.String())
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestAFailedBundleCopyRemovesTheDirectoryItCreated(t *testing.T) {
	bundle := t.TempDir()
	if err := os.WriteFile(filepath.Join(bundle, "summary.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	good := &run{opts: Options{Out: filepath.Join(t.TempDir(), "good")}, bundle: bundle}
	if _, err := good.copyBundle(); err != nil {
		t.Fatalf("canary: a readable bundle failed to copy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "locked.json"), []byte("{}"), 0o000); err != nil {
		t.Fatal(err)
	}
	if os.Getuid() == 0 {
		t.Skip("root reads a mode-000 file, so the copy cannot be made to fail this way")
	}
	bad := &run{opts: Options{Out: filepath.Join(t.TempDir(), "bad")}, bundle: bundle}
	if _, err := bad.copyBundle(); err == nil {
		t.Fatal("a copy of an unreadable file succeeded")
	}
	if _, err := os.Lstat(bad.opts.Out); !os.IsNotExist(err) {
		t.Fatalf("the partial --out directory stays: %v", err)
	}
}

func TestTheStageTailMasksAPlantedValue(t *testing.T) {
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	r := &run{env: &hubstages.Env{Masker: m}, lines: []string{"app said " + planted, "plain line"}}
	tail := strings.Join(r.tail(), "\n")
	if strings.Contains(tail, planted) || !strings.Contains(tail, "app said ****") || !strings.Contains(tail, "plain line") {
		t.Fatalf("tail %q", tail)
	}
}

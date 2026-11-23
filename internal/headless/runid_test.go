package headless

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
	{"a headless run never takes the id of a project on the host", "return no ids from takenRunIDs", "red: the run gets the id a container already carries", "TestTheRunIDSkipsIDsThatContainersNetworksOrClaimsHold"},
	{"two headless runs never claim the same id", "create the claim with os.MkdirAll instead of os.Mkdir", "red: concurrent runs share an id", "TestRunsThatClaimAtTheSameTimeNeverShareAnID"},
	{"a refused run gives its claim back", "drop the os.Remove of the claim after a refused prepare", "red: the claim directory of the refused run stays", "TestARunReleasesItsClaimWhetherItIsRefusedOrFinishes"},
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

var claimNow = time.Date(2026, 11, 23, 2, 0, 0, 0, time.UTC)

const claimSHA = "0123456789abcdef"

func TestTheRunIDSkipsIDsThatContainersNetworksOrClaimsHold(t *testing.T) {
	first := store.HeadlessRunID(claimNow, claimSHA, 1)
	id, claim, err := claimRunID(context.Background(), labelDocker{}, t.TempDir(), claimNow, claimSHA)
	if err != nil || id != first || !store.IsRunID(id) || filepath.Base(claim) != "cockpit-"+first {
		t.Fatalf("canary: an empty host gives %s %s %v", id, claim, err)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "cockpit-"+store.HeadlessRunID(claimNow, claimSHA, 2)), 0o700); err != nil {
		t.Fatal(err)
	}
	d := labelDocker{"ps": first + "\nrun-20260101-000000-aaaaaaa\n", "network": store.HeadlessRunID(claimNow, claimSHA, 3) + "\n"}
	id, claim, err = claimRunID(context.Background(), d, dir, claimNow, claimSHA)
	want := store.HeadlessRunID(claimNow, claimSHA, 4)
	if err != nil || id != want {
		t.Fatalf("id %s %v, want %s", id, err, want)
	}
	if info, err := os.Stat(claim); err != nil || !info.IsDir() || claim != filepath.Join(dir, "cockpit-"+want) {
		t.Fatalf("claim %s: %v", claim, err)
	}
}

func TestRunsThatClaimAtTheSameTimeNeverShareAnID(t *testing.T) {
	const runs = 32
	dir := t.TempDir()
	ids := make(chan string, runs)
	var wg sync.WaitGroup
	for i := 0; i < runs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, _, err := claimRunID(context.Background(), labelDocker{}, dir, claimNow, claimSHA)
			if err != nil {
				t.Error(err)
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Fatalf("two runs claimed %s", id)
		}
		seen[id] = true
	}
	if len(seen) != runs || !seen[store.HeadlessRunID(claimNow, claimSHA, runs)] {
		t.Fatalf("%d distinct ids from %d runs", len(seen), runs)
	}
}

func TestARunReleasesItsClaimWhetherItIsRefusedOrFinishes(t *testing.T) {
	project := toyProject(t)
	head := strings.TrimSpace(gitOut(t, project, "rev-parse", "HEAD"))
	held := "cockpit-" + store.HeadlessRunID(claimNow, head, 1)
	for _, c := range []struct {
		suite string
		code  int
	}{{"", cli.ExitOK}, {"absent", cli.ExitRejected}} {
		tmp := t.TempDir()
		if err := os.Mkdir(filepath.Join(tmp, held), 0o700); err != nil {
			t.Fatal(err)
		}
		f := newFake(nil)
		var out bytes.Buffer
		code := Run(context.Background(), Options{Project: project, Suite: c.suite}, Deps{
			Docker: f.For(procgroup.Hooks{}), EnsureImage: func(context.Context) (runnerbin.Image, error) { return fakeImage, nil },
			Home: t.TempDir(), Out: &out, Free: func(int) bool { return true }, TempDir: tmp, Submitter: "tester", ListenCanary: canaryFor(f), Now: func() time.Time { return claimNow },
		})
		if code != c.code || !strings.Contains(out.String(), "run "+store.HeadlessRunID(claimNow, head, 2)) {
			t.Fatalf("suite %q: exit %d, want %d, and the id after the held one\n%s", c.suite, code, c.code, out.String())
		}
		left, err := os.ReadDir(tmp)
		if err != nil || len(left) != 1 || left[0].Name() != held {
			t.Fatalf("suite %q: the temp dir holds %v %v", c.suite, left, err)
		}
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

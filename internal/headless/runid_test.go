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
	{"two headless runs never claim the same id", "treat every docker create result as a claim", "red: concurrent runs share an id", "TestRunsThatClaimAtTheSameTimeNeverShareAnID"},
	{"a run gives its claim back whether it is refused or finishes", "drop the docker rm of the claim container in release", "red: the claim container stays", "TestARunReleasesItsClaimWhetherItIsRefusedOrFinishes"},
	{"a failed bundle copy leaves no partial directory", "drop the RemoveAll after a failed fill in copyBundle", "red: the half-copied --out directory stays", "TestAFailedBundleCopyRemovesTheDirectoryItCreated"},
	{"the stage tail passes the run redactor", "return r.lines unmasked from tail", "red: the planted value shows in the tail", "TestTheStageTailMasksAPlantedValue"},
}

func TestRunIDMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "runid_test.go", runIDMutations)
}

type claimDocker struct {
	mu      sync.Mutex
	labels  map[string]string
	names   map[string]string
	refuse  string
	creates int
}

func newClaimDocker(labels map[string]string) *claimDocker {
	return &claimDocker{labels: labels, names: map[string]string{}}
}

func (d *claimDocker) Run(ctx context.Context, cmd compose.Command) (compose.Result, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if cmd.Args[0] != "create" {
		return compose.Result{Stdout: []byte(d.labels[cmd.Args[0]])}, nil
	}
	d.creates++
	name, label := cmd.Args[2], cmd.Args[4]
	if d.refuse != "" {
		return compose.Result{Code: 1, Stderr: []byte(d.refuse)}, nil
	}
	if _, ok := d.names[name]; ok {
		return compose.Result{Code: 125, Stderr: []byte(`Error response from daemon: Conflict. The container name "/` + name + `" is already in use by container "0123".`)}, nil
	}
	d.names[name] = label
	return compose.Result{}, nil
}

var claimNow = time.Date(2026, 11, 23, 2, 0, 0, 0, time.UTC)

const claimSHA = "0123456789abcdef"

func TestTheRunIDSkipsIDsThatContainersNetworksOrClaimsHold(t *testing.T) {
	first := store.HeadlessRunID(claimNow, claimSHA, 1)
	empty := newClaimDocker(nil)
	id, err := claimRunID(context.Background(), empty, "img", claimNow, claimSHA)
	if err != nil || id != first || !store.IsRunID(id) || empty.names[claimName(first)] != claimLabel+"="+first {
		t.Fatalf("canary: an empty host gives %s %v %v", id, err, empty.names)
	}
	d := newClaimDocker(map[string]string{"ps": first + "\nrun-20260101-000000-aaaaaaa\n", "network": store.HeadlessRunID(claimNow, claimSHA, 3) + "\n"})
	d.names[claimName(store.HeadlessRunID(claimNow, claimSHA, 2))] = "held"
	id, err = claimRunID(context.Background(), d, "img", claimNow, claimSHA)
	want := store.HeadlessRunID(claimNow, claimSHA, 4)
	if err != nil || id != want || d.names[claimName(want)] != claimLabel+"="+want || d.creates != 2 {
		t.Fatalf("id %s %v, want %s after 2 creates, made %d", id, err, want, d.creates)
	}
	broken := newClaimDocker(nil)
	broken.refuse = "Unable to find image 'img' locally"
	if id, err := claimRunID(context.Background(), broken, "img", claimNow, claimSHA); err == nil || broken.creates != 1 {
		t.Fatalf("a docker error other than a name conflict gave %s after %d creates", id, broken.creates)
	}
}

func TestRunsThatClaimAtTheSameTimeNeverShareAnID(t *testing.T) {
	const runs = 32
	d := newClaimDocker(nil)
	ids := make(chan string, runs)
	var wg sync.WaitGroup
	for i := 0; i < runs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := claimRunID(context.Background(), d, "img", claimNow, claimSHA)
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
	id := store.HeadlessRunID(claimNow, head, 1)
	for _, c := range []struct {
		free bool
		code int
	}{{true, cli.ExitOK}, {false, cli.ExitRejected}} {
		f := newFake(nil)
		var out bytes.Buffer
		code := Run(context.Background(), Options{Project: project}, Deps{
			Docker: f.For(procgroup.Hooks{}), EnsureImage: func(context.Context) (runnerbin.Image, error) { return fakeImage, nil },
			Home: t.TempDir(), Out: &out, Free: func(int) bool { return c.free }, TempDir: t.TempDir(), Submitter: "tester", ListenCanary: canaryFor(f), Now: func() time.Time { return claimNow },
		})
		created, removed := 0, 0
		for _, call := range f.Calls() {
			line := strings.Join(call.Args, " ")
			if strings.HasPrefix(line, "create --name "+claimName(id)+" ") {
				created++
			}
			if line == "rm -f "+claimName(id) {
				removed++
			}
		}
		if code != c.code || created != 1 || removed != 1 || !strings.Contains(out.String(), "run "+id) {
			t.Fatalf("free %v: exit %d, want %d; claim created %d, removed %d\n%s", c.free, code, c.code, created, removed, out.String())
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

package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/config"
	"github.com/d0lim/cockpit/internal/hub/lease"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var wiringMutations = []porttest.Mutation{
	{"hub config limits reach the pool", "return pool.DefaultLimits from ConfigLimits", "red: keptPool 2 and queueCap 4 do not arrive", "TestHubConfigLimitsPortBlocksAndReservedRangesReachThePoolAndTheLayout"},
	{"reserved ranges and the hub port reach the layout", "drop Reserved or HubPort from ConfigLayout", "red: a block over a reserved range or the hub port validates", "TestHubConfigLimitsPortBlocksAndReservedRangesReachThePoolAndTheLayout"},
	{"submissions pass the allowed repository, dirty, host API, suite and collector checks", "accept every submission in Accept", "red: an unlisted, dirty, hostApi 2, unknown-suite or runner-collector commit is queued", "TestSubmissionFromAnUnlistedRepositoryADirtyTreeAnUnservedHostAPIAnUnknownSuiteOrAReservedCollectorIsRefused"},
	{"a refused declaration stops at the submission and never reaches a run directory, tree extraction or docker", "ignore the ReadDeclaration error in Accept", "red: the refusal no longer names project.expectedServices", "TestARefusedDeclarationNeverReachesARunDirectoryTreeExtractionOrDocker"},
	{"the runner user reaches the generated harness while work/e2e and the private env stay owner-only", "drop RunnerUser from the stage Env", "red: the harness runner has no user", "TestRunnerUserReachesTheHarnessAndTheRunDirectoryStaysOwnerOnly"},
}

func TestWiringMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "wiring_test.go", wiringMutations)
}

func TestHubConfigLimitsPortBlocksAndReservedRangesReachThePoolAndTheLayout(t *testing.T) {
	cfg, err := config.Parse([]byte("limits:\n  keptPool: 2\n  runningCap: 1\n  queueCap: 4\nports:\n  base: 30000\n  slots: 3\nreservedPorts:\n  - { name: other-hub, from: 31000, to: 31100 }\n"))
	if err != nil {
		t.Fatal(err)
	}
	if l := ConfigLimits(cfg); l.PoolMax != 2 || l.UpCap != 1 || l.QueueMax != 4 {
		t.Fatalf("limits %+v", l)
	}
	layout := ConfigLayout(cfg, 7710)
	if layout.Base != 30000 || layout.Slots != 3 || layout.Stride != 100 || layout.HubPort != 7710 || len(layout.Reserved) != 1 || layout.Reserved[0] != (lease.Reserved{Name: "other-hub", From: 31000, To: 31100}) {
		t.Fatalf("layout %+v", layout)
	}
	if err := layout.Validate(); err != nil {
		t.Fatalf("a clear layout was refused: %v", err)
	}
	overlap, _ := config.Parse([]byte("ports:\n  base: 30000\n  slots: 3\nreservedPorts:\n  - { name: other-hub, from: 30150, to: 30160 }\n"))
	if _, _, err := New(Options{Home: t.TempDir(), Config: overlap, HubPort: 7710}); err == nil || !strings.Contains(err.Error(), "other-hub") {
		t.Fatalf("overlapping reserved range: %v", err)
	}
	hubInBlock, _ := config.Parse([]byte("ports:\n  base: 7700\n  slots: 1\n"))
	if err := ConfigLayout(hubInBlock, 7710).Validate(); err == nil {
		t.Fatal("a block holding the hub port validated")
	}
}

func refused(t *testing.T, w *world, worktree string) *ticker.Failure {
	t.Helper()
	r := w.sched.Submit(t.Context(), scheduler.SubmitInput{Worktree: worktree, Submitter: "tester"})
	return r.Fail
}

func TestSubmissionFromAnUnlistedRepositoryADirtyTreeAnUnservedHostAPIAnUnknownSuiteOrAReservedCollectorIsRefused(t *testing.T) {
	w := newWorld(t, nil, nil)
	other := toyRepo(t, nil)
	if f := refused(t, w, other); f == nil || f.Code != ticker.CodeRejectedWorktree {
		t.Errorf("unlisted repository: %+v", f)
	}
	os.WriteFile(filepath.Join(w.repo, "mock", "main.go"), []byte("package main\n"), 0o644)
	if f := refused(t, w, w.repo); f == nil || f.Code != ticker.CodeRejectedDirty || len(f.Files) != 1 {
		t.Errorf("dirty tree: %+v", f)
	}
	gitIn(t, w.repo, "checkout", "--", "mock/main.go")
	api := newWorld(t, nil, func(d string) { replaceIn(t, filepath.Join(d, ".cockpit", "project.yaml"), "hostApi: 1", "hostApi: 2") })
	if f := refused(t, api, api.repo); f == nil || f.Code != ticker.CodeRejectedHostAPI || !strings.Contains(f.Reason, "unknown host API version 2") {
		t.Errorf("host API 2: %+v", f)
	}
	collector := newWorld(t, nil, func(d string) {
		os.WriteFile(filepath.Join(d, "scripts", "collectors", "logs.ts"), []byte("export {};\n"), 0o644)
	})
	if f := refused(t, collector, collector.repo); f == nil || f.Code != ticker.CodeRejectedNoHarness || !strings.Contains(f.Reason, "reserved for the hub") {
		t.Errorf("reserved collector: %+v", f)
	}
	r := w.sched.Submit(t.Context(), scheduler.SubmitInput{Worktree: w.repo, Suite: "nightly", Submitter: "tester"})
	if r.Fail == nil || r.Fail.Code != ticker.CodeRejectedNoHarness || !strings.Contains(r.Fail.Reason, "no suite nightly") {
		t.Errorf("unknown suite: %+v", r.Fail)
	}
	ok := w.sched.Submit(t.Context(), scheduler.SubmitInput{Worktree: filepath.Join(w.repo, "mock"), Submitter: "tester"})
	if ok.Fail != nil {
		t.Fatalf("a clean submission from a subdirectory was refused: %+v", ok.Fail)
	}
	m := w.meta(ok.ID)
	common, _ := filepath.EvalSymlinks(filepath.Join(w.repo, ".git"))
	if m.Suite != "smoke" || m.Repository != common || len(m.SHA) != 40 {
		t.Fatalf("queued run %+v", m)
	}
}

func TestARefusedDeclarationNeverReachesARunDirectoryTreeExtractionOrDocker(t *testing.T) {
	w := newWorld(t, nil, func(d string) {
		replaceIn(t, filepath.Join(d, ".cockpit", "project.yaml"), "expectedServices: [app, db, cache, seed]", "expectedServices: [app, db, queue, seed]")
	})
	f := refused(t, w, w.repo)
	if f == nil || f.Code != ticker.CodeRejectedNoHarness || !strings.Contains(f.Reason, "project.expectedServices[2]") {
		t.Fatalf("refusal %+v", f)
	}
	st, err := w.hub.Store().CurrentState()
	if err != nil {
		t.Fatal(err)
	}
	runs, _ := os.ReadDir(filepath.Join(w.hub.Store().Home(), "runs"))
	if len(st.Queue) != 0 || len(st.Leases) != 0 || len(runs) != 0 {
		t.Fatalf("queue %v, leases %v, run directories %d", st.Queue, st.Leases, len(runs))
	}
	if verbs := w.verbs(); len(verbs) != 0 {
		t.Fatalf("the refused submission reached docker: %v", verbs)
	}
}

func harnessRunnerUser(t *testing.T, dir string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "harness.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if err := yamlUnmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	user, ok := doc.Services["runner"]["user"].(string)
	return user, ok
}

func TestRunnerUserReachesTheHarnessAndTheRunDirectoryStaysOwnerOnly(t *testing.T) {
	evaluated := 0
	for _, user := range []string{"1000:1000", ""} {
		w := newWorldAs(t, user, nil, nil)
		m := w.submit()
		dir, _ := w.hub.Store().RunDir(m.ID)
		got, ok := harnessRunnerUser(t, dir)
		if user != "" && got != user || user == "" && ok != (stages.HostRunnerUser() != "") {
			t.Errorf("RunnerUser %q: harness user %q %v", user, got, ok)
		}
		for path, want := range map[string]os.FileMode{filepath.Join(dir, "work", "e2e"): 0o700, filepath.Join(dir, "src", "e2e.private.env.json"): 0o600} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != want {
				t.Errorf("%s: %v %v", path, info, err)
			}
		}
		evaluated++
	}
	if evaluated != 2 {
		t.Fatalf("evaluated %d", evaluated)
	}
}

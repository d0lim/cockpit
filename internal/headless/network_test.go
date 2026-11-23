package headless

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/cli"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/seal"
)

var networkMutations = []porttest.Mutation{
	{"the report reads the level from seal.json", "drop Network from the report inputs in ReportBundle", "red: an open run's summary has no network and no not-proven line", "TestAnOpenCommitSkipsTheProbesAndItsBundleSaysIsolationWasNotProven"},
	{"the unseal check refuses an open commit", "drop the open check in the declaration step", "red: --unseal-check on an open commit starts the run", "TestTheUnsealCheckRefusesAnOpenCommitAndAcceptsASealedOne"},
}

func TestNetworkMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "network_test.go", networkMutations)
}

func openProject(t *testing.T) string {
	t.Helper()
	dir := toyProject(t)
	path := filepath.Join(dir, compose.DeclarationPath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, []byte("network: open\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", dir, "-c", "user.name=toy", "-c", "user.email=toy@example.invalid", "-c", "maintenance.auto=false", "-c", "gc.auto=0", "commit", "-qam", "open")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	return dir
}

func TestAnOpenCommitSkipsTheProbesAndItsBundleSaysIsolationWasNotProven(t *testing.T) {
	sealedOut := filepath.Join(t.TempDir(), "sealed")
	if code, text := runFake(t, toyProject(t), newFake(nil), Options{Out: sealedOut}); code != cli.ExitOK || !strings.Contains(text, "probe 4/4 blocked") {
		t.Fatalf("canary: a sealed run exit %d\n%s", code, text)
	}
	if s := readJSON(t, filepath.Join(sealedOut, "summary.json")); s["network"] != "sealed" {
		t.Fatalf("sealed summary network %v", s["network"])
	}
	out := filepath.Join(t.TempDir(), "open")
	code, text := runFake(t, openProject(t), newFake(nil), Options{Out: out})
	if code != cli.ExitOK || !strings.Contains(text, "network open: the probes did not run") || strings.Contains(text, "probe 4/4") {
		t.Fatalf("open run exit %d\n%s", code, text)
	}
	summary := readJSON(t, filepath.Join(out, "summary.json"))
	declared, _ := summary["notProven"].(map[string]any)["declared"].([]any)
	if summary["network"] != "open" || summary["verdict"] != "passed" || !slices.Contains(declared, any(report.OpenNetworkNotProven)) {
		t.Fatalf("open summary network %v verdict %v declared %v", summary["network"], summary["verdict"], declared)
	}
	sealFile := readJSON(t, filepath.Join(out, seal.FileName))
	if probes, _ := sealFile["probes"].([]any); sealFile["network"] != "open" || len(probes) != 0 {
		t.Fatalf("seal.json network %v probes %v", sealFile["network"], sealFile["probes"])
	}
}

func TestTheUnsealCheckRefusesAnOpenCommitAndAcceptsASealedOne(t *testing.T) {
	code, text := runFake(t, openProject(t), newFake(nil), Options{UnsealCheck: true})
	if code != cli.ExitRejected || !strings.Contains(text, "--unseal-check needs network level sealed") {
		t.Fatalf("open with --unseal-check: exit %d\n%s", code, text)
	}
	if code, text := runFake(t, toyProject(t), newFake(nil), Options{UnsealCheck: true}); code == cli.ExitRejected {
		t.Fatalf("canary: a sealed commit with --unseal-check was refused\n%s", text)
	}
}

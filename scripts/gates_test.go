package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(wd)
}

func runGate(t *testing.T, root, script, target string, env ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join(root, "scripts", script), target)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

var scanned = regexp.MustCompile(`scanned (\d+) files`)

func scannedCount(t *testing.T, out string) int {
	t.Helper()
	m := scanned.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("missing scanned line: %q", out)
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func plantName(t *testing.T) string {
	t.Helper()
	planted := t.TempDir()
	dir := filepath.Join(planted, "internal", "x")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("package x\n\nconst base = \"http://acme-mock:9095\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return planted
}

func TestConsumerNameCheckPassesCleanTreeAndReportsScanCount(t *testing.T) {
	out, err := runGate(t, repoRoot(t), "check-no-consumer-names.sh", repoRoot(t), "COCKPIT_CONSUMER_NAMES=acme")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if n := scannedCount(t, out); n == 0 {
		t.Fatalf("scanned file count is 0: %q", out)
	}
	if !strings.Contains(out, "names ok") {
		t.Fatalf("missing pass marker: %q", out)
	}
}

func TestConsumerNameCheckRejectsPlantedMatchingNameWithItsPath(t *testing.T) {
	out, err := runGate(t, repoRoot(t), "check-no-consumer-names.sh", plantName(t), "COCKPIT_CONSUMER_NAMES=acme")
	if err == nil {
		t.Fatalf("planted name passed:\n%s", out)
	}
	if n := scannedCount(t, out); n != 1 {
		t.Fatalf("scanned file count %d: %q", n, out)
	}
	if !strings.Contains(out, "x.go") || !strings.Contains(out, "acme-mock") {
		t.Fatalf("path and name not reported: %q", out)
	}
}

func TestConsumerNameCheckOnlyScansAndSkipsWhenPatternIsEmpty(t *testing.T) {
	out, err := runGate(t, repoRoot(t), "check-no-consumer-names.sh", plantName(t), "COCKPIT_CONSUMER_NAMES=")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if n := scannedCount(t, out); n != 1 {
		t.Fatalf("scanned file count %d: %q", n, out)
	}
	if !strings.Contains(out, "names skipped") || strings.Contains(out, "names ok") {
		t.Fatalf("missing skip marker: %q", out)
	}
}

func TestConsumerNameCheckFailsWhenNoFilesToScan(t *testing.T) {
	out, err := runGate(t, repoRoot(t), "check-no-consumer-names.sh", t.TempDir(), "COCKPIT_CONSUMER_NAMES=")
	if err == nil {
		t.Fatalf("empty tree passed:\n%s", out)
	}
	if n := scannedCount(t, out); n != 0 {
		t.Fatalf("scanned file count %d: %q", n, out)
	}
}

func TestLicenseGateRejectsK6RequireByModuleName(t *testing.T) {
	root := repoRoot(t)
	planted := t.TempDir()
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	mod = append(mod, []byte("\nrequire go.k6.io/k6 v1.0.0\n")...)
	if err := os.WriteFile(filepath.Join(planted, "go.mod"), mod, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runGate(t, root, "check-licenses.sh", planted)
	if err == nil {
		t.Fatalf("k6 require passed:\n%s", out)
	}
	if !strings.Contains(out, "denied module: go.k6.io/k6") {
		t.Fatalf("denial reason does not name the module: %q", out)
	}
}

func TestLicenseGatePassesCleanTree(t *testing.T) {
	out, err := runGate(t, repoRoot(t), "check-licenses.sh", repoRoot(t))
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "licenses ok") {
		t.Fatalf("missing pass marker: %q", out)
	}
}

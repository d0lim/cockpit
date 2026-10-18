package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
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

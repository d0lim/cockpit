package scripts

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func buildNpm(t *testing.T, version string) (string, string, error) {
	t.Helper()
	root := repoRoot(t)
	out := filepath.Join(t.TempDir(), "npm")
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "build-npm.sh"), version, out)
	cmd.Dir = root
	log, err := cmd.CombinedOutput()
	return out, string(log), err
}

func TestNpmPackageIsAssembledFromTheGeneratedSdkWithTheGivenVersion(t *testing.T) {
	root := repoRoot(t)
	out, log, err := buildNpm(t, "1.2.3")
	if err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{"README.md", "cockpit-collect.d.ts", "cockpit-flow.d.ts", "cockpit-globals.d.ts", "index.d.ts", "package.json", "tsconfig.base.json"}
	if !slices.Equal(names, want) {
		t.Fatalf("files %v, want %v", names, want)
	}
	for _, name := range []string{"cockpit-collect.d.ts", "cockpit-flow.d.ts", "cockpit-globals.d.ts", "tsconfig.base.json"} {
		generated, _ := os.ReadFile(filepath.Join(root, "sdk", name))
		packaged, _ := os.ReadFile(filepath.Join(out, name))
		if len(generated) == 0 || !bytes.Equal(generated, packaged) {
			t.Fatalf("%s differs from sdk/ or is empty", name)
		}
	}
	pkg, _ := os.ReadFile(filepath.Join(out, "package.json"))
	for _, s := range []string{`"name": "@d0lim/cockpit-sdk"`, `"version": "1.2.3"`, `"types": "./index.d.ts"`} {
		if !strings.Contains(string(pkg), s) {
			t.Fatalf("package.json lacks %s:\n%s", s, pkg)
		}
	}
	index, _ := os.ReadFile(filepath.Join(out, "index.d.ts"))
	if strings.Count(string(index), "/// <reference path=") != 3 {
		t.Fatalf("index.d.ts %q", index)
	}
}

func TestNpmPackageBuildRefusesAVersionThatIsNotSemver(t *testing.T) {
	for _, bad := range []string{"v1.2.3", "1.2", "latest", ""} {
		out, log, err := buildNpm(t, bad)
		if err == nil || !strings.Contains(log, "version") {
			t.Fatalf("version %q was accepted: %v\n%s", bad, err, log)
		}
		if _, statErr := os.Stat(out); statErr == nil {
			t.Fatalf("version %q left an output directory behind", bad)
		}
	}
}

func TestNpmPackagePassesBunPackDryRun(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Fatalf("bun not found; run under mise exec: %v", err)
	}
	out, log, err := buildNpm(t, "1.2.3")
	if err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	cmd := exec.Command(bun, "pm", "pack", "--dry-run")
	cmd.Dir = out
	packed, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bun pm pack: %v\n%s", err, packed)
	}
	entries, _ := os.ReadDir(out)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tgz") {
			t.Fatalf("a dry run wrote %s", e.Name())
		}
	}
}

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
	want := []string{"GUIDE.md", "README.md", "cockpit-collect.d.ts", "cockpit-flow.d.ts", "cockpit-globals.d.ts", "index.d.ts", "package.json", "tsconfig.base.json"}
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

func fakeNpm(t *testing.T, script string) (string, string) {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "calls.log")
	body := "#!/usr/bin/env bash\necho \"$*\" >> " + log + "\n" + script
	if err := os.WriteFile(filepath.Join(bin, "npm"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

func publish(t *testing.T, npmScript string) (string, string, error) {
	t.Helper()
	out, buildLog, err := buildNpm(t, "1.2.3")
	if err != nil {
		t.Fatalf("%v\n%s", err, buildLog)
	}
	bin, log := fakeNpm(t, npmScript)
	cmd := exec.Command("bash", filepath.Join(repoRoot(t), "scripts", "publish-npm.sh"), out)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	stdout, err := cmd.CombinedOutput()
	calls, _ := os.ReadFile(log)
	return string(stdout), string(calls), err
}

func TestNpmPublishSkipsWithANoticeWhenThePackageIsNotOnTheRegistryYet(t *testing.T) {
	out, calls, err := publish(t, "if [ \"$1\" = view ]; then echo 'npm error code E404' >&2; exit 1; fi\n")
	if err != nil || !strings.Contains(out, "not on the registry yet") || strings.Contains(calls, "publish") {
		t.Fatalf("err %v\nout %s\ncalls %s", err, out, calls)
	}
}

func TestNpmPublishSkipsAVersionThatIsAlreadyPublished(t *testing.T) {
	out, calls, err := publish(t, "if [ \"$1\" = view ]; then echo 1.2.3; fi\n")
	if err != nil || !strings.Contains(out, "already published") || strings.Contains(calls, "publish") {
		t.Fatalf("err %v\nout %s\ncalls %s", err, out, calls)
	}
}

func TestNpmPublishPublishesANewVersionOfAnExistingPackage(t *testing.T) {
	out, calls, err := publish(t, "if [ \"$1\" = view ] && [ \"$2\" = @d0lim/cockpit-sdk ]; then echo 1.2.2; fi\n")
	if err != nil || !strings.Contains(calls, "publish --access public") || !strings.Contains(out, "published @d0lim/cockpit-sdk@1.2.3") {
		t.Fatalf("err %v\nout %s\ncalls %s", err, out, calls)
	}
}

func TestNpmPublishFailsOnARegistryErrorOtherThanNotFound(t *testing.T) {
	out, calls, err := publish(t, "if [ \"$1\" = view ]; then echo 'npm error code ETIMEDOUT' >&2; exit 1; fi\n")
	if err == nil || !strings.Contains(out, "ETIMEDOUT") || strings.Contains(calls, "publish") {
		t.Fatalf("err %v\nout %s\ncalls %s", err, out, calls)
	}
}

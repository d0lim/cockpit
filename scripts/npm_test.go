package scripts

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	want := []string{"GUIDE.md", "HUB.md", "README.md", "cockpit-collect.d.ts", "cockpit-flow.d.ts", "cockpit-globals.d.ts", "declaration.schema.json", "index.d.ts", "package.json", "tsconfig.base.json"}
	if !slices.Equal(names, want) {
		t.Fatalf("files %v, want %v", names, want)
	}
	copies := map[string]string{
		"cockpit-collect.d.ts":    "sdk/cockpit-collect.d.ts",
		"cockpit-flow.d.ts":       "sdk/cockpit-flow.d.ts",
		"cockpit-globals.d.ts":    "sdk/cockpit-globals.d.ts",
		"tsconfig.base.json":      "sdk/tsconfig.base.json",
		"declaration.schema.json": "schema/declaration.json",
	}
	for name, source := range copies {
		generated, _ := os.ReadFile(filepath.Join(root, source))
		packaged, _ := os.ReadFile(filepath.Join(out, name))
		if len(generated) == 0 || !bytes.Equal(generated, packaged) {
			t.Fatalf("%s differs from %s or is empty", name, source)
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

func TestPackagedGuidesLinkEachOtherInThePackageAndEveryOtherDocOnGitHub(t *testing.T) {
	out, log, err := buildNpm(t, "1.2.3")
	if err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	link := regexp.MustCompile(`\]\(([^)#\s]+)`)
	checked, local, remote := 0, 0, 0
	for _, name := range []string{"GUIDE.md", "HUB.md"} {
		text, _ := os.ReadFile(filepath.Join(out, name))
		for _, m := range link.FindAllStringSubmatch(string(text), -1) {
			checked++
			target := m[1]
			switch {
			case strings.HasPrefix(target, "https://github.com/d0lim/cockpit/blob/main/docs/"):
				remote++
				doc := strings.TrimPrefix(target, "https://github.com/d0lim/cockpit/blob/main/docs/")
				if _, err := os.Stat(filepath.Join(repoRoot(t), "docs", doc)); err != nil {
					t.Fatalf("%s links %s, which docs/ does not hold", name, target)
				}
			case strings.Contains(target, "://"):
			default:
				local++
				if _, err := os.Stat(filepath.Join(out, target)); err != nil {
					t.Fatalf("%s links %s, which the package does not hold", name, target)
				}
			}
		}
	}
	if local == 0 || remote == 0 {
		t.Fatalf("checked %d links: %d inside the package, %d on GitHub", checked, local, remote)
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

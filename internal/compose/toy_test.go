package compose

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/engine"
)

const typescriptPackage = "typescript@5.9.3"

func toyScripts(t *testing.T, sub string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(toyDir(t), "scripts", sub, "*.ts"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func TestToySuiteBundlesAtTheRunnerSiteAndEachCollectorAtTheHubSite(t *testing.T) {
	ctx := context.Background()
	suites := toyScripts(t, "suites")
	collectors := toyScripts(t, "collectors")
	if len(suites) != 1 || len(collectors) != 3 {
		t.Fatalf("toy has %d suites and %d collectors", len(suites), len(collectors))
	}
	for _, s := range suites {
		b, err := engine.BuildBundle(ctx, engine.BundleInput{Entry: s, Root: filepath.Join(toyDir(t), "scripts"), Site: engine.SiteRunner})
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		for _, id := range []string{`"health"`, `"orders"`, `"facts"`} {
			if !strings.Contains(b.Code, id) {
				t.Errorf("%s bundle lacks flow %s", s, id)
			}
		}
	}
	for _, c := range collectors {
		if _, err := engine.BuildBundle(ctx, engine.BundleInput{Entry: c, Root: toyDir(t), Site: engine.SiteHub}); err != nil {
			t.Errorf("%s: %v", c, err)
		}
	}
	if _, err := engine.BuildBundle(ctx, engine.BundleInput{Entry: collectors[0], Root: toyDir(t), Site: engine.SiteRunner}); err == nil {
		t.Fatal("a collector bundled at the runner site")
	}
}

var tscFileLine = regexp.MustCompile(`(?m)^(.+\.ts)$`)

func TestToyScriptsTypeCheckAgainstTheGeneratedDeclarations(t *testing.T) {
	bunx, err := exec.LookPath("bunx")
	if err != nil {
		t.Fatalf("bunx not found; run under mise exec: %v", err)
	}
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join(toyDir(t), "scripts"))); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(repoRoot(t), "sdk", "tsconfig.base.json")
	config, err := json.Marshal(map[string]any{"extends": base, "include": []string{"**/*.ts"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), config, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bunx, "--bun", "-p", typescriptPackage, "tsc", "--noEmit", "-p", filepath.Join(dir, "tsconfig.json"), "--listFiles", "--pretty", "false")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tsc: %v\n%s", err, out)
	}
	checked := 0
	real, _ := filepath.EvalSymlinks(dir)
	for _, m := range tscFileLine.FindAllString(string(out), -1) {
		if strings.HasPrefix(m, dir+"/") || strings.HasPrefix(m, real+"/") {
			checked++
		}
	}
	if want := len(toyScripts(t, "suites")) + len(toyScripts(t, "flows")) + len(toyScripts(t, "collectors")); checked != want || want == 0 {
		t.Fatalf("tsc checked %d toy files, want %d\n%s", checked, want, out)
	}
}

func TestToyMakeEnvWritesItsDeclaredOutputsWithoutPrintingValues(t *testing.T) {
	dir := t.TempDir()
	script := readFile(t, filepath.Join(toyDir(t), "make-env.sh"))
	writeFile(t, filepath.Join(dir, "make-env.sh"), script)
	cmd := exec.Command("/bin/sh", "-c", `umask 077 && exec sh make-env.sh`)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil || len(out) != 0 {
		t.Fatalf("make-env.sh: %v, printed %q", err, out)
	}
	for _, p := range toyDeclaration(t).Env.Outputs {
		if mode(t, filepath.Join(dir, string(p))) != 0o600 {
			t.Errorf("%s mode %v", p, mode(t, filepath.Join(dir, string(p))))
		}
	}
	env := readFile(t, filepath.Join(dir, ".env"))
	if !regexp.MustCompile(`(?m)^APP_TOKEN=toy_[A-Za-z0-9]{24}$`).MatchString(env) || !regexp.MustCompile(`(?m)^POSTGRES_PASSWORD=[A-Za-z0-9]{32}$`).MatchString(env) {
		t.Fatalf(".env has unexpected shapes")
	}
	var priv struct {
		Local map[string]string `json:"local"`
	}
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, PrivateEnvName))), &priv); err != nil || !strings.Contains(env, "APP_TOKEN="+priv.Local["appToken"]+"\n") {
		t.Fatalf("private env does not carry the app token: %v", err)
	}
}

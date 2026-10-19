package contract

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func copyRepo(t *testing.T) string {
	t.Helper()
	src, dst := repoRoot(t), t.TempDir()
	skip := map[string]bool{".git": true, "node_modules": true}
	rootOnly := map[string]bool{"bin": true, "dist": true}
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return err
		}
		if skip[d.Name()] || rootOnly[rel] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dst, ".git")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("copy contains .git: %v", err)
	}
	return dst
}

func isolatedEnv(dir string) []string {
	return []string{
		"GIT_DIR=" + filepath.Join(dir, ".git"),
		"GIT_WORK_TREE=" + dir,
		"GIT_CEILING_DIRECTORIES=" + filepath.Dir(dir),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_COUNT=4",
		"GIT_CONFIG_KEY_0=maintenance.auto",
		"GIT_CONFIG_VALUE_0=false",
		"GIT_CONFIG_KEY_1=gc.auto",
		"GIT_CONFIG_VALUE_1=0",
		"GIT_CONFIG_KEY_2=maintenance.autoDetach",
		"GIT_CONFIG_VALUE_2=false",
		"GIT_CONFIG_KEY_3=gc.autoDetach",
		"GIT_CONFIG_VALUE_3=false",
		"GIT_AUTHOR_NAME=contract-test",
		"GIT_AUTHOR_EMAIL=contract-test@example.invalid",
		"GIT_COMMITTER_NAME=contract-test",
		"GIT_COMMITTER_EMAIL=contract-test@example.invalid",
		"GOWORK=off",
		"GOTOOLCHAIN=local",
	}
}

func commitAll(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "base"}} {
		if out, err := command(t, dir, isolatedEnv(dir), "git", args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func replaceOnce(t *testing.T, path, old, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), old); n != 1 {
		t.Fatalf("%s contains %q %d times", path, old, n)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), old, replacement, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateDiffCheckIsGreenOnACleanTreeAfterGenerating(t *testing.T) {
	t.Parallel()
	dir := copyRepo(t)
	commitAll(t, dir)
	removed := filepath.Join(dir, "sdk", "cockpit-flow.d.ts")
	if err := os.Remove(removed); err != nil {
		t.Fatal(err)
	}
	if out, err := command(t, dir, isolatedEnv(dir), "make", "generate-check"); err != nil {
		t.Fatalf("red on a clean tree: %v\n%s", err, out)
	}
	if _, err := os.Stat(removed); err != nil {
		t.Fatalf("deleted generated file was not recreated: %v", err)
	}
	if out, err := command(t, dir, isolatedEnv(dir), "sh", "-c", "go generate ./... && git diff --exit-code"); err != nil {
		t.Fatalf("diff left on a clean tree: %v\n%s", err, out)
	}
	out, err := command(t, dir, isolatedEnv(dir), "git", "ls-files", "--", "schema", "sdk")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(strings.Fields(out)); n != len(Roots())+len(Modules())+2 {
		t.Fatalf("%d tracked generated files\n%s", n, out)
	}
}

func TestGenerateDiffCheckTurnsRedWhenOneStructLineChanges(t *testing.T) {
	t.Parallel()
	dir := copyRepo(t)
	commitAll(t, dir)
	replaceOnce(t, filepath.Join(dir, "internal", "contract", "signal.go"), `json:"value"`, `json:"amount"`)
	out, err := command(t, dir, isolatedEnv(dir), "make", "generate-check")
	if err == nil {
		t.Fatalf("green after changing a struct\n%s", out)
	}
	for _, drifted := range []string{"schema/signal.json", "sdk/cockpit-collect.d.ts"} {
		if !strings.Contains(out, drifted) {
			t.Fatalf("drift in %s was not reported\n%s", drifted, out)
		}
	}
}

func TestHandEditedGeneratedLineMakesGitDiffFailAfterGoGenerate(t *testing.T) {
	t.Parallel()
	dir := copyRepo(t)
	replaceOnce(t, filepath.Join(dir, "sdk", "cockpit-flow.d.ts"), "sleep(ms: number): Promise<void>;", "sleep(ms: number): Promise<number>;")
	commitAll(t, dir)
	out, err := command(t, dir, isolatedEnv(dir), "sh", "-c", "go generate ./... && git diff --exit-code")
	if err == nil {
		t.Fatalf("hand-edited generated file passed without a diff\n%s", out)
	}
	if !strings.Contains(out, "sdk/cockpit-flow.d.ts") || !strings.Contains(out, "Promise<void>") {
		t.Fatalf("diff does not point at the edited line\n%s", out)
	}
}

func TestGeneratedSchemasNeverAllowNull(t *testing.T) {
	scanned := 0
	for _, r := range Roots() {
		data, err := os.ReadFile(filepath.Join(repoRoot(t), "schema", r.Name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), `"null"`) {
			t.Errorf("schema/%s.json allows null", r.Name)
		}
		if !strings.Contains(string(data), `"type"`) {
			t.Fatalf("schema/%s.json has no type", r.Name)
		}
		scanned++
	}
	if scanned != len(Roots()) || scanned == 0 {
		t.Fatalf("checked %d schemas", scanned)
	}
}

func TestGeneratedDirectoriesHoldOnlyFilesNamedByRootsAndModules(t *testing.T) {
	var schemas, sdk []string
	for _, r := range Roots() {
		schemas = append(schemas, r.Name+".json")
	}
	for _, m := range Modules() {
		sdk = append(sdk, m.File)
	}
	sdk = append(sdk, "tsconfig.base.json", GlobalsFile)
	for dir, want := range map[string][]string{"schema": schemas, "sdk": sdk} {
		entries, err := os.ReadDir(filepath.Join(repoRoot(t), dir))
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, e := range entries {
			got = append(got, e.Name())
		}
		if !slices.Equal(got, sorted(want...)) {
			t.Fatalf("files in %s/ %v, want %v", dir, got, sorted(want...))
		}
	}
	if len(schemas) != 9 || len(sdk) != 4 {
		t.Fatalf("%d roots, %d sdk files", len(schemas), len(sdk))
	}
}

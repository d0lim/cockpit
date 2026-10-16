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
	skip := map[string]bool{".git": true, "bin": true, "dist": true, "node_modules": true}
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return err
		}
		if skip[d.Name()] {
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
		t.Fatalf("사본에 .git 이 들어왔다: %v", err)
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
		t.Fatalf("%s 에 %q 가 %d 번 있다", path, old, n)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), old, replacement, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func Test깨끗한_트리에서는_생성_뒤_생성_diff_검사가_초록이다(t *testing.T) {
	t.Parallel()
	dir := copyRepo(t)
	commitAll(t, dir)
	removed := filepath.Join(dir, "sdk", "cockpit-flow.d.ts")
	if err := os.Remove(removed); err != nil {
		t.Fatal(err)
	}
	if out, err := command(t, dir, isolatedEnv(dir), "make", "generate-check"); err != nil {
		t.Fatalf("깨끗한 트리에서 빨갛다: %v\n%s", err, out)
	}
	if _, err := os.Stat(removed); err != nil {
		t.Fatalf("지운 생성물이 다시 생기지 않았다: %v", err)
	}
	if out, err := command(t, dir, isolatedEnv(dir), "sh", "-c", "go generate ./... && git diff --exit-code"); err != nil {
		t.Fatalf("깨끗한 트리에서 diff 가 남았다: %v\n%s", err, out)
	}
	out, err := command(t, dir, isolatedEnv(dir), "git", "ls-files", "--", "schema", "sdk")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(strings.Fields(out)); n != len(Roots())+len(Modules())+2 {
		t.Fatalf("추적된 생성물 %d 개\n%s", n, out)
	}
}

func Test구조체_한_줄을_바꾸면_생성_diff_검사가_빨개진다(t *testing.T) {
	t.Parallel()
	dir := copyRepo(t)
	commitAll(t, dir)
	replaceOnce(t, filepath.Join(dir, "internal", "contract", "signal.go"), `json:"value"`, `json:"amount"`)
	out, err := command(t, dir, isolatedEnv(dir), "make", "generate-check")
	if err == nil {
		t.Fatalf("구조체를 바꿨는데 초록이다\n%s", out)
	}
	for _, drifted := range []string{"schema/signal.json", "sdk/cockpit-collect.d.ts"} {
		if !strings.Contains(out, drifted) {
			t.Fatalf("%s 의 드리프트가 보고되지 않았다\n%s", drifted, out)
		}
	}
}

func Test생성물_한_줄을_손으로_바꾼_사본은_go_generate_뒤_git_diff가_실패한다(t *testing.T) {
	t.Parallel()
	dir := copyRepo(t)
	replaceOnce(t, filepath.Join(dir, "sdk", "cockpit-flow.d.ts"), "sleep(ms: number): Promise<void>;", "sleep(ms: number): Promise<number>;")
	commitAll(t, dir)
	out, err := command(t, dir, isolatedEnv(dir), "sh", "-c", "go generate ./... && git diff --exit-code")
	if err == nil {
		t.Fatalf("손으로 바꾼 생성물이 diff 없이 지났다\n%s", out)
	}
	if !strings.Contains(out, "sdk/cockpit-flow.d.ts") || !strings.Contains(out, "Promise<void>") {
		t.Fatalf("diff 가 바꾼 줄을 짚지 않는다\n%s", out)
	}
}

func Test생성한_스키마는_어느_자리에도_null을_허용하지_않는다(t *testing.T) {
	scanned := 0
	for _, r := range Roots() {
		data, err := os.ReadFile(filepath.Join(repoRoot(t), "schema", r.Name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), `"null"`) {
			t.Errorf("schema/%s.json 이 null 을 허용한다", r.Name)
		}
		if !strings.Contains(string(data), `"type"`) {
			t.Fatalf("schema/%s.json 에 type 이 없다", r.Name)
		}
		scanned++
	}
	if scanned != len(Roots()) || scanned == 0 {
		t.Fatalf("검사한 스키마 %d 개", scanned)
	}
}

func Test생성물_디렉토리는_루트와_모듈이_정한_파일만_든다(t *testing.T) {
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
			t.Fatalf("%s/ 의 파일 %v, 기대 %v", dir, got, sorted(want...))
		}
	}
	if len(schemas) != 9 || len(sdk) != 4 {
		t.Fatalf("루트 %d 개, sdk 파일 %d 개", len(schemas), len(sdk))
	}
}

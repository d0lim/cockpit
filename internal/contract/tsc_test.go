package contract

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
)

const typescriptPackage = "typescript@5.9.3"

type tscError struct {
	file    string
	code    string
	message string
}

type tscRun struct {
	err     error
	output  string
	samples []string
	sdk     []string
	errors  []tscError
}

var tscErrorLine = regexp.MustCompile(`^(.+)\(\d+,\d+\): error (TS\d+): (.*)$`)

func within(t *testing.T, dir, path string) bool {
	t.Helper()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{dir, real} {
		if strings.HasPrefix(path, prefix+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func runTsc(t *testing.T, samples ...string) tscRun {
	t.Helper()
	bunx, err := exec.LookPath("bunx")
	if err != nil {
		t.Fatalf("bunx 를 찾지 못했다 — mise exec 아래에서 돌린다: %v", err)
	}
	dir := t.TempDir()
	for _, name := range samples {
		data, err := os.ReadFile(filepath.Join("testdata", "tsc", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base := filepath.Join(repoRoot(t), "sdk", "tsconfig.base.json")
	config, err := json.Marshal(map[string]any{"extends": base, "include": []string{"*.ts"}})
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
	run := tscRun{err: err, output: string(out)}
	sdkDir := filepath.Join(repoRoot(t), "sdk")
	for _, line := range strings.Split(run.output, "\n") {
		line = strings.TrimSpace(line)
		if m := tscErrorLine.FindStringSubmatch(line); m != nil {
			run.errors = append(run.errors, tscError{file: filepath.Base(m[1]), code: m[2], message: m[3]})
			continue
		}
		switch {
		case within(t, dir, line) && strings.HasSuffix(line, ".ts"):
			run.samples = append(run.samples, filepath.Base(line))
		case within(t, sdkDir, line) && strings.HasSuffix(line, ".d.ts"):
			run.sdk = append(run.sdk, filepath.Base(line))
		}
	}
	return run
}

func Test생성한_선언으로_올바른_플로우와_수집_스크립트가_tsc를_지난다(t *testing.T) {
	run := runTsc(t, "flow-ok.ts", "collect-ok.ts")
	if run.err != nil || len(run.errors) > 0 {
		t.Fatalf("tsc 실패: %v\n%s", run.err, run.output)
	}
	if len(run.samples) != 2 || len(run.sdk) != 3 {
		t.Fatalf("검사한 샘플 %v, 선언 %v\n%s", run.samples, run.sdk, run.output)
	}
}

func Test러너_자리에서_ctx_sql을_부르거나_예약_값을_어긴_스크립트는_tsc에서_걸린다(t *testing.T) {
	run := runTsc(t, "flow-uses-sql.ts", "collect-bad-values.ts", "flow-uses-fetch.ts")
	if run.err == nil {
		t.Fatalf("tsc 가 통과했다\n%s", run.output)
	}
	if len(run.samples) != 3 || len(run.sdk) != 3 {
		t.Fatalf("검사한 샘플 %v, 선언 %v\n%s", run.samples, run.sdk, run.output)
	}
	want := []tscError{
		{"flow-uses-sql.ts", "TS2339", "Property 'sql' does not exist on type 'Ctx'"},
		{"flow-uses-fetch.ts", "TS2304", "Cannot find name 'fetch'"},
		{"collect-bad-values.ts", "TS2322", `"purple"`},
		{"collect-bad-values.ts", "TS2322", `"done"`},
	}
	if len(run.errors) != len(want) {
		t.Fatalf("오류 %d 건, 기대 %d 건\n%s", len(run.errors), len(want), run.output)
	}
	for _, w := range want {
		found := false
		for _, e := range run.errors {
			found = found || (e.file == w.file && e.code == w.code && strings.Contains(e.message, w.message))
		}
		if !found {
			t.Fatalf("기대한 오류가 없다: %+v\n%s", w, run.output)
		}
	}
}

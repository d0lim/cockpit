package sdkbin

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestWrittenFilesMatchEmbeddedSdkInCountNamesAndContent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sdk")
	written, err := Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	names, err := Names()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(written, names) || !slices.Equal(names, []string{"cockpit-collect.d.ts", "cockpit-flow.d.ts", "cockpit-globals.d.ts", "tsconfig.base.json"}) {
		t.Fatalf("written %v names %v", written, names)
	}
	for _, name := range names {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("..", "..", "sdk", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s differs from sdk/ in the repository", name)
		}
	}
	if _, err := Write(dir); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
}

func runTsc(t *testing.T, sdkDir string, sample string) (bool, string, int) {
	t.Helper()
	bunx, err := exec.LookPath("bunx")
	if err != nil {
		t.Fatalf("bunx not found; run under mise exec: %v", err)
	}
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "contract", "testdata", "tsc", sample))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sample), data, 0o644); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(map[string]any{"extends": filepath.Join(sdkDir, "tsconfig.base.json"), "include": []string{"*.ts"}})
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), config, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	real, _ := filepath.EvalSymlinks(sdkDir)
	for attempt := 1; ; attempt++ {
		cmd := exec.CommandContext(ctx, bunx, "--bun", "-p", "typescript@5.9.3", "tsc", "--noEmit", "-p", filepath.Join(dir, "tsconfig.json"), "--listFiles", "--pretty", "false")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		declarations := 0
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasSuffix(line, ".d.ts") && (strings.HasPrefix(line, sdkDir) || strings.HasPrefix(line, real)) {
				declarations++
			}
		}
		if declarations > 0 || attempt == 3 {
			return err == nil, string(out), declarations
		}
		t.Logf("tsc listed no declarations, treated as a bunx startup failure; retry %d\n%s", attempt, out)
	}
}

func TestTscOnWrittenSdkAcceptsValidFlowAndRejectsFlowCallingSql(t *testing.T) {
	sdkDir := filepath.Join(t.TempDir(), "sdk")
	if _, err := Write(sdkDir); err != nil {
		t.Fatal(err)
	}
	ok, out, decls := runTsc(t, sdkDir, "flow-ok.ts")
	if !ok || decls != 3 {
		t.Fatalf("valid sample: ok=%v decls=%d\n%s", ok, decls, out)
	}
	ok, out, decls = runTsc(t, sdkDir, "flow-uses-sql.ts")
	if ok || decls != 3 || !strings.Contains(out, "TS2339") {
		t.Fatalf("invalid sample: ok=%v decls=%d\n%s", ok, decls, out)
	}
}

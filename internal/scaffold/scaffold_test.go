package scaffold

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/runner"
	"github.com/d0lim/cockpit/internal/sdkbin"
)

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInitWritesTheQuickStartFilesWithTheirModes(t *testing.T) {
	dir := t.TempDir()
	written, err := Write(dir, "http://web:8080/")
	if err != nil || len(written) != 7 {
		t.Fatalf("written %v err %v", written, err)
	}
	if got := read(t, filepath.Join(dir, ".cockpit/project.yaml")); got != "hostApi: 1\n" {
		t.Fatalf("declaration %q", got)
	}
	if got := read(t, filepath.Join(dir, "e2e/env.json")); !strings.Contains(got, `"baseUrl": "http://web:8080"`) {
		t.Fatalf("env %q", got)
	}
	if got := read(t, filepath.Join(dir, "e2e/suites/smoke.ts")); !strings.Contains(got, `name: "smoke"`) {
		t.Fatalf("suite %q", got)
	}
	for path, want := range map[string]os.FileMode{".e2e/e2e.private.env.json": 0o600, ".e2e": 0o777, "compose.cockpit.yaml": 0o644} {
		info, err := os.Stat(filepath.Join(dir, path))
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s: mode %v err %v", path, info.Mode().Perm(), err)
		}
	}
}

func TestInitRefusesToOverwriteAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "e2e"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "e2e/env.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Write(dir, DefaultBaseURL)
	if err == nil || !strings.Contains(err.Error(), "e2e/env.json") {
		t.Fatalf("err %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "compose.cockpit.yaml")); err == nil {
		t.Fatal("a file was written despite the refusal")
	}
	if got := read(t, filepath.Join(dir, "e2e/env.json")); got != "{}" {
		t.Fatalf("existing file changed to %q", got)
	}
}

func TestInitRejectsABaseURLThatIsNotAnOriginAndWritesNothing(t *testing.T) {
	for _, bad := range []string{"http://web/api", "http://10.0.0.1", "web", "ftp://web"} {
		dir := t.TempDir()
		if _, err := Write(dir, bad); err == nil || !strings.Contains(err.Error(), "not an origin") {
			t.Fatalf("%s: err %v", bad, err)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Fatalf("%s: wrote %v", bad, entries)
		}
	}
}

func runScaffold(t *testing.T, status int) (int, string) {
	t.Helper()
	dir := t.TempDir()
	if _, err := Write(dir, "http://web"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(status)
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(srv.Close)
	env := map[string]string{
		"COCKPIT_SCRIPTS":     filepath.Join(dir, "e2e"),
		"COCKPIT_DECLARATION": filepath.Join(dir, ".cockpit/project.yaml"),
		"E2E_DIR":             filepath.Join(dir, ".e2e"),
		"COCKPIT_SOURCES":     filepath.Join(dir, "source"),
	}
	var out, errOut strings.Builder
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	code := runner.Main(ctx, []string{"smoke"}, func(k string) string { return env[k] }, runner.Deps{
		Stdout: &out,
		Stderr: &errOut,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
		},
	})
	return code, out.String() + errOut.String()
}

func TestScaffoldedSuitePassesTheRunnerWhenTheServiceAnswers200(t *testing.T) {
	if code, out := runScaffold(t, 200); code != 0 || !strings.Contains(out, "home › get passed") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestScaffoldedSuiteFailsTheRunnerWhenTheServiceAnswers500(t *testing.T) {
	if code, out := runScaffold(t, 500); code != 1 || !strings.Contains(out, "home › get failed") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func tsc(t *testing.T, suite string) (bool, string) {
	t.Helper()
	bunx, err := exec.LookPath("bunx")
	if err != nil {
		t.Fatalf("bunx not found; run under mise exec: %v", err)
	}
	dir := t.TempDir()
	if _, err := Write(dir, DefaultBaseURL); err != nil {
		t.Fatal(err)
	}
	if suite != "" {
		if err := os.WriteFile(filepath.Join(dir, "e2e/suites/smoke.ts"), []byte(suite), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pkg := filepath.Join(dir, "node_modules/@d0lim/cockpit-sdk")
	if _, err := sdkbin.Write(pkg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"name":"@d0lim/cockpit-sdk","version":"0.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bunx, "--bun", "-p", "typescript@5.9.3", "tsc", "--noEmit", "-p", filepath.Join(dir, "e2e/tsconfig.json"), "--pretty", "false")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return err == nil, string(out)
}

func TestScaffoldedSuiteTypeChecksAgainstTheGeneratedDeclarations(t *testing.T) {
	if ok, out := tsc(t, ""); !ok {
		t.Fatalf("tsc failed:\n%s", out)
	}
}

func TestScaffoldedTsconfigRejectsAWrongHelperArgument(t *testing.T) {
	ok, out := tsc(t, strings.Replace(smokeSuite, "expectStatus(res, 200)", `expectStatus(res, "200")`, 1))
	if ok || !strings.Contains(out, "TS2345") {
		t.Fatalf("ok=%v\n%s", ok, out)
	}
}

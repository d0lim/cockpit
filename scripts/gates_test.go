package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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

var scanned = regexp.MustCompile(`scanned (\d+) files`)

func scannedCount(t *testing.T, out string) int {
	t.Helper()
	m := scanned.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("scanned 줄이 없다: %q", out)
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func plantName(t *testing.T) string {
	t.Helper()
	planted := t.TempDir()
	dir := filepath.Join(planted, "internal", "x")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("package x\n\nconst base = \"http://acme-mock:9095\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return planted
}

func Test이름_검사는_깨끗한_트리를_통과시키고_스캔_수를_낸다(t *testing.T) {
	out, err := runGate(t, repoRoot(t), "check-no-consumer-names.sh", repoRoot(t), "COCKPIT_CONSUMER_NAMES=acme")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if n := scannedCount(t, out); n == 0 {
		t.Fatalf("스캔 파일 수 0: %q", out)
	}
	if !strings.Contains(out, "names ok") {
		t.Fatalf("통과 표지가 없다: %q", out)
	}
}

func Test이름_검사는_패턴에_걸리는_심은_이름을_경로와_함께_거부한다(t *testing.T) {
	out, err := runGate(t, repoRoot(t), "check-no-consumer-names.sh", plantName(t), "COCKPIT_CONSUMER_NAMES=acme")
	if err == nil {
		t.Fatalf("심은 이름이 통과했다:\n%s", out)
	}
	if n := scannedCount(t, out); n != 1 {
		t.Fatalf("스캔 파일 수 %d: %q", n, out)
	}
	if !strings.Contains(out, "x.go") || !strings.Contains(out, "acme-mock") {
		t.Fatalf("경로와 이름이 보고되지 않았다: %q", out)
	}
}

func Test이름_검사는_패턴이_비어_있으면_스캔만_하고_건너뛴다(t *testing.T) {
	out, err := runGate(t, repoRoot(t), "check-no-consumer-names.sh", plantName(t), "COCKPIT_CONSUMER_NAMES=")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if n := scannedCount(t, out); n != 1 {
		t.Fatalf("스캔 파일 수 %d: %q", n, out)
	}
	if !strings.Contains(out, "names skipped") || strings.Contains(out, "names ok") {
		t.Fatalf("건너뜀 표지가 없다: %q", out)
	}
}

func Test이름_검사는_스캔할_파일이_없으면_실패한다(t *testing.T) {
	out, err := runGate(t, repoRoot(t), "check-no-consumer-names.sh", t.TempDir(), "COCKPIT_CONSUMER_NAMES=")
	if err == nil {
		t.Fatalf("빈 트리가 통과했다:\n%s", out)
	}
	if n := scannedCount(t, out); n != 0 {
		t.Fatalf("스캔 파일 수 %d: %q", n, out)
	}
}

func Test라이선스_게이트는_k6_require를_모듈_이름으로_거부한다(t *testing.T) {
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
		t.Fatalf("k6 require 가 통과했다:\n%s", out)
	}
	if !strings.Contains(out, "denied module: go.k6.io/k6") {
		t.Fatalf("거부 사유가 모듈 이름을 짚지 않는다: %q", out)
	}
}

func Test라이선스_게이트는_깨끗한_트리를_통과시킨다(t *testing.T) {
	out, err := runGate(t, repoRoot(t), "check-licenses.sh", repoRoot(t))
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "licenses ok") {
		t.Fatalf("통과 표지가 없다: %q", out)
	}
}

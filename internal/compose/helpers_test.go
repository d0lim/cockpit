package compose

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

const planted = "PLANTED-secret-7f3a9c"

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func toyDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "testdata", "toy-project")
}

func plainGitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env,
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=toy", "GIT_AUTHOR_EMAIL=toy@example.invalid",
		"GIT_COMMITTER_NAME=toy", "GIT_COMMITTER_EMAIL=toy@example.invalid",
	)
}

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = plainGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func toyRepo(t *testing.T, mutate func(dir string)) (string, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.CopyFS(dir, os.DirFS(toyDir(t))); err != nil {
		t.Fatal(err)
	}
	for _, generated := range []string{".env", PrivateEnvName, "mock/bin"} {
		os.RemoveAll(filepath.Join(dir, generated))
	}
	if mutate != nil {
		mutate(dir)
	}
	gitT(t, dir, "init", "-q", "-b", "main")
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-q", "-m", "toy")
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real, gitT(t, dir, "rev-parse", "HEAD")
}

func commit(t *testing.T, dir string, mutate func()) string {
	t.Helper()
	mutate()
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-q", "-m", "change")
	return gitT(t, dir, "rev-parse", "HEAD")
}

func replaceIn(t *testing.T, path, old, new string) {
	t.Helper()
	text := readFile(t, path)
	if !strings.Contains(text, old) {
		t.Fatalf("%s does not contain %q", path, old)
	}
	writeFile(t, path, strings.Replace(text, old, new, 1))
}

func loadToy(t *testing.T, mutate func(dir string)) (Loaded, error) {
	t.Helper()
	dir, sha := toyRepo(t, mutate)
	return ReadDeclaration(context.Background(), Git{Dir: dir}, sha)
}

func mustLoadToy(t *testing.T) Loaded {
	t.Helper()
	loaded, err := loadToy(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func toyDeclaration(t *testing.T) contract.Declaration {
	t.Helper()
	decl, err := ParseDeclaration([]byte(readFile(t, filepath.Join(toyDir(t), DeclarationPath))))
	if err != nil {
		t.Fatal(err)
	}
	return decl
}

func rejection(t *testing.T, err error) *Rejection {
	t.Helper()
	var r *Rejection
	if !errors.As(err, &r) {
		t.Fatalf("not a rejection: %v", err)
	}
	return r
}

func testLayout(t *testing.T) Layout {
	t.Helper()
	l, err := NewLayout(filepath.Join(t.TempDir(), "run"))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

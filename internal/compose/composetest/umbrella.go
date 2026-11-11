package composetest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func Env() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env,
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_COUNT=5",
		"GIT_CONFIG_KEY_0=maintenance.auto", "GIT_CONFIG_VALUE_0=false", "GIT_CONFIG_KEY_1=gc.auto", "GIT_CONFIG_VALUE_1=0",
		"GIT_CONFIG_KEY_2=maintenance.autoDetach", "GIT_CONFIG_VALUE_2=false", "GIT_CONFIG_KEY_3=gc.autoDetach", "GIT_CONFIG_VALUE_3=false",
		"GIT_CONFIG_KEY_4=protocol.file.allow", "GIT_CONFIG_VALUE_4=always",
		"GIT_AUTHOR_NAME=toy", "GIT_AUTHOR_EMAIL=toy@example.invalid",
		"GIT_COMMITTER_NAME=toy", "GIT_COMMITTER_EMAIL=toy@example.invalid",
	)
}

func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = Env()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func Submodule(t testing.TB, repo, path, name string) string {
	t.Helper()
	origin := filepath.Join(t.TempDir(), "origin")
	if err := os.Rename(filepath.Join(repo, filepath.FromSlash(path)), origin); err != nil {
		t.Fatal(err)
	}
	Git(t, origin, "init", "-q", "-b", "main")
	Git(t, origin, "add", "-A")
	Git(t, origin, "commit", "-q", "-m", "submodule "+name)
	Git(t, repo, "rm", "-r", "-q", "--cached", "--ignore-unmatch", "--", path)
	Git(t, repo, "submodule", "add", "-q", "--name", name, "--", origin, path)
	Git(t, repo, "commit", "-q", "-m", "add submodule "+name)
	return origin
}

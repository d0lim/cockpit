package config_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/config"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var allowMutations = []porttest.Mutation{
	{"only repositories in allowedRepositories are accepted", "return the submitted common dir without comparing it", "red: an unlisted repository and a nested repository pass", "TestWorktreesOfAnAllowedRepositoryPassAndUnlistedNestedOrNonGitPathsAreRejected"},
	{"entries and submissions match through the git common dir whatever path reaches it", "compare the cleaned submitted path with the entry instead of the common dirs", "red: a linked worktree, a subdirectory and an entry given through a symlink no longer match", "TestAllowedEntryGivenThroughASymlinkAdmitsTheRepositoryByItsRealPath"},
	{"allowed entries and submissions are absolute", "drop the IsAbs checks", "red: a relative entry loads and a relative submission is resolved against the hub cwd", "TestRelativeAllowedEntriesAndRelativeSubmissionsAreRejected"},
}

func TestAllowMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "allow_test.go", allowMutations)
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_COUNT=4", "GIT_CONFIG_KEY_0=maintenance.auto", "GIT_CONFIG_VALUE_0=false", "GIT_CONFIG_KEY_1=gc.auto", "GIT_CONFIG_VALUE_1=0", "GIT_CONFIG_KEY_2=maintenance.autoDetach", "GIT_CONFIG_VALUE_2=false", "GIT_CONFIG_KEY_3=gc.autoDetach", "GIT_CONFIG_VALUE_3=false",
		"GIT_AUTHOR_NAME=toy", "GIT_AUTHOR_EMAIL=toy@example.invalid", "GIT_COMMITTER_NAME=toy", "GIT_COMMITTER_EMAIL=toy@example.invalid")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func repo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "file.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func TestWorktreesOfAnAllowedRepositoryPassAndUnlistedNestedOrNonGitPathsAreRejected(t *testing.T) {
	base := t.TempDir()
	allowed := repo(t, filepath.Join(base, "allowed"))
	worktree := filepath.Join(base, "linked")
	git(t, allowed, "worktree", "add", "-q", worktree)
	other := repo(t, filepath.Join(base, "other"))
	nested := repo(t, filepath.Join(allowed, "nested"))
	plain := filepath.Join(base, "plain")
	if err := os.Mkdir(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AllowedRepositories: []string{allowed}}
	ctx := context.Background()
	pass := []string{allowed, filepath.Join(allowed, "sub"), worktree, filepath.Join(worktree, "sub")}
	deny := []string{other, nested, filepath.Join(nested, "sub"), plain}
	var common string
	for _, p := range pass {
		got, err := cfg.Allow(ctx, compose.CommonDirOf, p)
		if err != nil {
			t.Errorf("%s was rejected: %v", p, err)
		}
		if common == "" {
			common = got
		} else if got != common {
			t.Errorf("%s resolved to %s, others to %s", p, got, common)
		}
	}
	denied := 0
	for _, p := range deny {
		_, err := cfg.Allow(ctx, compose.CommonDirOf, p)
		if !errors.Is(err, config.ErrNotAllowed) || !strings.Contains(err.Error(), p) {
			t.Errorf("%s: %v", p, err)
		}
		denied++
	}
	if denied != len(deny) || common == "" {
		t.Fatalf("denied %d of %d, common %q", denied, len(deny), common)
	}
}

func TestAllowedEntryGivenThroughASymlinkAdmitsTheRepositoryByItsRealPath(t *testing.T) {
	base := t.TempDir()
	allowed := repo(t, filepath.Join(base, "allowed"))
	link := filepath.Join(base, "via-link")
	if err := os.Symlink(allowed, link); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(base, "linked")
	git(t, link, "worktree", "add", "-q", worktree)
	ctx := context.Background()
	evaluated := 0
	for _, entry := range []string{allowed, link} {
		cfg := config.Config{AllowedRepositories: []string{entry}}
		for _, p := range []string{filepath.Join(allowed, "sub"), worktree} {
			got, err := cfg.Allow(ctx, compose.CommonDirOf, p)
			real, _ := filepath.EvalSymlinks(filepath.Join(allowed, ".git"))
			if err != nil || got != real {
				t.Errorf("entry %s, submission %s: %q %v, want %s", entry, p, got, err, real)
			}
			evaluated++
		}
	}
	if evaluated != 4 {
		t.Fatalf("evaluated %d cases", evaluated)
	}
}

func TestRelativeAllowedEntriesAndRelativeSubmissionsAreRejected(t *testing.T) {
	if _, err := config.Parse([]byte("allowedRepositories:\n  - src/toy\n")); err == nil || !strings.Contains(err.Error(), "allowedRepositories[0]") {
		t.Fatalf("relative entry: %v", err)
	}
	if _, err := config.Parse([]byte("allowedRepositories:\n  - /src/toy\n")); err != nil {
		t.Fatalf("absolute entry was rejected: %v", err)
	}
	allowed := repo(t, filepath.Join(t.TempDir(), "allowed"))
	cfg := config.Config{AllowedRepositories: []string{allowed}}
	if _, err := cfg.Allow(context.Background(), compose.CommonDirOf, "allowed"); !errors.Is(err, config.ErrNotAllowed) {
		t.Fatalf("relative submission: %v", err)
	}
	if _, err := cfg.Allow(context.Background(), compose.CommonDirOf, allowed); err != nil {
		t.Fatalf("absolute submission was rejected: %v", err)
	}
}

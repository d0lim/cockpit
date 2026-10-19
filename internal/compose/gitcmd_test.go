package compose

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var gitMutations = []porttest.Mutation{
	{"repository fsmonitor commands do not run", "drop -c core.fsmonitor=false from HardenedArgs", "red: the planted fsmonitor marker appears after Dirty", "TestDirtyCheckDoesNotRunAPlantedFsmonitorCommandThatPlainGitRuns"},
	{"repository hooks do not run", "drop -c core.hooksPath from HardenedArgs", "red: the planted pre-commit marker appears after a hub commit", "TestHooksPlantedInTheRepositoryDoNotRunUnderTheHubGitAndDoUnderPlainGit"},
	{"inherited GIT_ variables do not redirect git", "pass os.Environ() unfiltered in gitEnv", "red: GIT_DIR from the hub environment makes the hub read another repository", "TestInheritedGitDirDoesNotRedirectHubGitButRedirectsPlainGit"},
	{"hub git starts no background maintenance that outlives the call", "drop the maintenance.auto and gc.auto settings from HardenedArgs", "red: the hub commit traces git maintenance run --auto --detach", "TestHubGitAndTestGitStartNoDetachedMaintenanceWhilePlainGitCommitDoes"},
	{"commit input resolves only to a commit", "drop the ^{commit} suffix in ResolveCommit", "red: HEAD:mock resolves to a tree id and is accepted as a commit", "TestResolveCommitRejectsEmptyAndOptionLikeInputAndResolvesRefsToFullShas"},
	{"untracked files count as dirty whatever the repository config says", "drop --untracked-files=normal from Dirty", "red: scripts/new.ts is not listed with status.showUntrackedFiles=no", "TestDirtyListsTrackedEditsAndUntrackedFilesUnderDeclaredPathsOnlyEvenWhenUntrackedDisplayIsOff"},
	{"links in the commit tree are visible to confinement", "make Tree.Link always return false", "red: linked/main.go is not reported as passing through a link", "TestTreeListingKnowsLinksDirectoriesAndFileCounts"},
}

func TestGitMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "gitcmd_test.go", gitMutations)
}

func plantScript(t *testing.T, path, marker string) {
	t.Helper()
	writeFile(t, path, "#!/bin/sh\ntouch '"+marker+"'\n")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestDirtyCheckDoesNotRunAPlantedFsmonitorCommandThatPlainGitRuns(t *testing.T) {
	dir, _ := toyRepo(t, nil)
	marker := filepath.Join(t.TempDir(), "fsmonitor-ran")
	monitor := filepath.Join(t.TempDir(), "monitor.sh")
	plantScript(t, monitor, marker)
	gitT(t, dir, "config", "core.fsmonitor", monitor)
	gitT(t, dir, "status", "--porcelain")
	if !exists(marker) {
		t.Fatal("canary: plain git status did not run the planted fsmonitor command")
	}
	os.Remove(marker)
	if _, err := (Git{Dir: dir}).Dirty(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if exists(marker) {
		t.Fatal("the hub dirty check ran the planted fsmonitor command")
	}
}

func TestHooksPlantedInTheRepositoryDoNotRunUnderTheHubGitAndDoUnderPlainGit(t *testing.T) {
	dir, _ := toyRepo(t, nil)
	marker := filepath.Join(t.TempDir(), "hook-ran")
	plantScript(t, filepath.Join(dir, ".hooks", "pre-commit"), marker)
	gitT(t, dir, "config", "core.hooksPath", ".hooks")
	gitT(t, dir, "commit", "-q", "--allow-empty", "-m", "canary")
	if !exists(marker) {
		t.Fatal("canary: plain git commit did not run the planted hook")
	}
	os.Remove(marker)
	cmd := exec.Command("git", HardenedArgs(dir, t.TempDir(), "commit", "-q", "--allow-empty", "-m", "hub")...)
	cmd.Env = plainGitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if exists(marker) {
		t.Fatal("the planted hook ran under the hardened arguments")
	}
	if _, err := (Git{Dir: dir}).Dirty(context.Background(), nil); err != nil || exists(marker) {
		t.Fatalf("dirty check: err %v, hook ran %v", err, exists(marker))
	}
}

func tracedCommit(t *testing.T, dir string, args []string, env []string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = append(env, "GIT_TRACE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func spawnsMaintenance(trace string) bool {
	return strings.Contains(trace, "maintenance run") || strings.Contains(trace, "gc --auto")
}

func TestHubGitAndTestGitStartNoDetachedMaintenanceWhilePlainGitCommitDoes(t *testing.T) {
	dir, _ := toyRepo(t, nil)
	var bare []string
	for _, kv := range plainGitEnv() {
		if !strings.HasPrefix(kv, "GIT_CONFIG_COUNT=") && !strings.HasPrefix(kv, "GIT_CONFIG_KEY_") && !strings.HasPrefix(kv, "GIT_CONFIG_VALUE_") {
			bare = append(bare, kv)
		}
	}
	commit := []string{"commit", "-q", "--allow-empty", "-m", "traced"}
	plain := tracedCommit(t, dir, append([]string{"-C", dir}, commit...), bare)
	if !spawnsMaintenance(plain) {
		t.Fatalf("canary: a plain git commit started no maintenance:\n%s", plain)
	}
	if hub := tracedCommit(t, dir, HardenedArgs(dir, t.TempDir(), commit...), bare); spawnsMaintenance(hub) {
		t.Fatalf("a commit under HardenedArgs started maintenance:\n%s", hub)
	}
	if test := tracedCommit(t, dir, append([]string{"-C", dir}, commit...), plainGitEnv()); spawnsMaintenance(test) {
		t.Fatalf("a commit under the test git environment started maintenance:\n%s", test)
	}
}

func TestInheritedGitDirDoesNotRedirectHubGitButRedirectsPlainGit(t *testing.T) {
	dir, _ := toyRepo(t, nil)
	other, otherSHA := toyRepo(t, func(d string) { writeFile(t, filepath.Join(d, "other.txt"), "other\n") })
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	plain := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	plain.Env = os.Environ()
	out, err := plain.Output()
	if err != nil || strings.TrimSpace(string(out)) != otherSHA {
		t.Fatalf("canary: plain git with GIT_DIR did not read the other repository: %q %v", out, err)
	}
	got, err := (Git{Dir: dir}).ResolveCommit(context.Background(), "HEAD")
	if err != nil || got == otherSHA {
		t.Fatalf("hub git followed GIT_DIR: %s %v", got, err)
	}
}

func TestResolveCommitRejectsEmptyAndOptionLikeInputAndResolvesRefsToFullShas(t *testing.T) {
	dir, sha := toyRepo(t, nil)
	g := Git{Dir: dir}
	for _, ref := range []string{"HEAD", "main", sha[:10], sha} {
		got, err := g.ResolveCommit(context.Background(), ref)
		if err != nil || got != sha {
			t.Errorf("%q resolved to %q, %v", ref, got, err)
		}
	}
	bad := []string{"", "  ", "--output=/tmp/x", " -h", "no-such-ref", "HEAD:mock", "HEAD^{tree}"}
	for _, ref := range bad {
		if _, err := g.ResolveCommit(context.Background(), ref); !errors.Is(err, ErrCommitInput) {
			t.Errorf("%q: %v", ref, err)
		}
	}
}

func TestDirtyListsTrackedEditsAndUntrackedFilesUnderDeclaredPathsOnlyEvenWhenUntrackedDisplayIsOff(t *testing.T) {
	dir, _ := toyRepo(t, nil)
	gitT(t, dir, "config", "status.showUntrackedFiles", "no")
	g := Git{Dir: dir}
	paths := []string{"mock", "scripts"}
	clean, err := g.Dirty(context.Background(), paths)
	if err != nil || len(clean) != 0 {
		t.Fatalf("fresh repository is dirty: %v %v", clean, err)
	}
	writeFile(t, filepath.Join(dir, "mock", "main.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "scripts", "new.ts"), "export {};\n")
	writeFile(t, filepath.Join(dir, "notes.txt"), "outside\n")
	writeFile(t, filepath.Join(dir, ".env"), "IGNORED=1\n")
	got, err := (Git{Dir: filepath.Join(dir, "mock")}).Dirty(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if strings.Join(got, ",") != "mock/main.go,scripts/new.ts" {
		t.Fatalf("dirty files %v", got)
	}
}

func TestTreeListingKnowsLinksDirectoriesAndFileCounts(t *testing.T) {
	dir, sha := toyRepo(t, func(d string) {
		if err := os.Symlink("mock", filepath.Join(d, "linked")); err != nil {
			t.Fatal(err)
		}
	})
	tree, err := (Git{Dir: dir}).ListTree(context.Background(), sha)
	if err != nil {
		t.Fatal(err)
	}
	if link, ok := tree.Link("linked/main.go"); !ok || link != "linked" {
		t.Fatalf("link %q %v", link, ok)
	}
	if _, ok := tree.Link("mock/main.go"); ok {
		t.Fatal("mock/main.go was reported as passing through a link")
	}
	if !tree.IsDir("mock") || tree.IsDir("mock/main.go") || !tree.Has(DeclarationPath) {
		t.Fatal("tree kinds are wrong")
	}
	files := strings.Fields(gitT(t, dir, "ls-files"))
	if tree.Files() != len(files) || len(files) == 0 {
		t.Fatalf("tree counts %d files, ls-files %d", tree.Files(), len(files))
	}
}

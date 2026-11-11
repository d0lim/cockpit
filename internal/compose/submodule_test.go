package compose

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose/composetest"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var submoduleMutations = []porttest.Mutation{
	{"a submodule name cannot climb out of modules/", "drop the cleanRelative name check in objects.module and resolve the name with os.Stat on the joined path", "red: the gitlink is spliced from the outside repository through a ../ name", "TestASubmoduleNameThatClimbsOutOfModulesOrLinksOutIsRejectedAndANestedNameOrAnInsideLinkPasses"},
	{"a link under modules/ cannot lead out of it", "resolve the name with os.Stat on the joined path instead of os.Root", "red: modules/evil linking to the outside repository is spliced", "TestASubmoduleNameThatClimbsOutOfModulesOrLinksOutIsRejectedAndANestedNameOrAnInsideLinkPasses"},
	{"a gitlink path is a clean relative path", "drop the cleanRelative check on the gitlink path in repository.submodule", "red: gitlinks at .. and . are spliced", "TestAGitlinkAtADotOrParentSegmentIsRejectedEvenWhenGitmodulesMapsItToAnInitializedSubmodule"},
	{"an include in a committed .gitmodules is not followed", "drop --no-includes from objects.config", "red: the gitlink maps through the planted host file and is spliced", "TestAnIncludeInACommittedGitmodulesIsNotFollowedAndItsEntriesDoNotMapAGitlink"},
	{"a gitlink that does not resolve fails the listing", "skip a gitlink whose module or pinned commit cannot be read in splice", "red: the listing passes and mock/ holds no files", "TestAnUninitializedSubmoduleOrAPinMissingFromItsModuleIsRejectedAtListingAndAnInitializedOneIsSpliced"},
	{"submodules resolve from the common dir, never from a linked worktree", "list the root through the worktree's own git dir (rev-parse --absolute-git-dir) instead of CommonDir", "red: a pin held only by worktrees/wt/modules/mock is spliced", "TestASubmoduleInitializedOnlyInALinkedWorktreeIsRejectedWithTheMainCheckoutHintUntilTheMainModuleHoldsThePin"},
	{"object reads never follow a module's core.worktree", "drop --git-dir=. --work-tree=. from objectsOnly", "red: git cannot chdir to the removed checkout", "TestSubmoduleObjectsAreReadAfterTheCheckoutIsRemovedWhilePlainGitInTheModuleFails"},
	{"a commit pins at most 64 submodules", "drop the maxSubmodules check in repository.submodule", "red: 65 submodules are spliced", "TestACommitPinningMoreThan64SubmodulesIsRejectedAtTheGitlinkThatCrossesTheCapAnd64Pass"},
}

func TestSubmoduleMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "submodule_test.go", submoduleMutations)
}

func umbrella(t *testing.T) (string, string) {
	t.Helper()
	dir, _ := toyRepo(t, nil)
	origin, err := filepath.EvalSymlinks(composetest.Submodule(t, dir, "mock", "mock"))
	if err != nil {
		t.Fatal(err)
	}
	return dir, origin
}

func listHead(t *testing.T, dir string) (Tree, error) {
	t.Helper()
	return Git{Dir: dir}.ListTree(context.Background(), gitT(t, dir, "rev-parse", "HEAD"))
}

func renameSubmodule(t *testing.T, dir, name string) {
	t.Helper()
	replaceIn(t, filepath.Join(dir, gitmodulesPath), `[submodule "mock"]`, fmt.Sprintf("[submodule %q]", name))
	gitT(t, dir, "commit", "-q", "-m", "rename", "--", gitmodulesPath)
}

func refusedAt(t *testing.T, err error, field, says string) {
	t.Helper()
	if r := rejection(t, err); r.Kind != RejectSubmodule || r.Field != field || !strings.Contains(r.Message, says) {
		t.Fatalf("want a submodule rejection at %s saying %q, got %v", field, says, r)
	}
}

func spliced(t *testing.T, tree Tree, err error, file string) {
	t.Helper()
	if err != nil {
		t.Fatalf("the listing failed: %v", err)
	}
	if e, ok := tree.Entries[file]; !ok || e.repo != "mock" {
		t.Fatalf("%s is not spliced from mock: %+v %v", file, e, ok)
	}
	if _, err := tree.Blob(context.Background(), file); err != nil {
		t.Fatalf("%s is not readable: %v", file, err)
	}
}

func TestASubmoduleNameThatClimbsOutOfModulesOrLinksOutIsRejectedAndANestedNameOrAnInsideLinkPasses(t *testing.T) {
	climb, origin := umbrella(t)
	modules := filepath.Join(climb, ".git", modulesDir)
	up, err := filepath.Rel(modules, filepath.Join(origin, ".git"))
	if err != nil || !strings.HasPrefix(up, "..") {
		t.Fatalf("relative name %q %v", up, err)
	}
	renameSubmodule(t, climb, filepath.ToSlash(up))
	_, err = listHead(t, climb)
	refusedAt(t, err, "mock", "clean relative path")

	link, origin := umbrella(t)
	if err := os.Symlink(filepath.Join(origin, ".git"), filepath.Join(link, ".git", modulesDir, "evil")); err != nil {
		t.Fatal(err)
	}
	renameSubmodule(t, link, "evil")
	_, err = listHead(t, link)
	refusedAt(t, err, "mock", "resolves outside")

	nested, _ := umbrella(t)
	modules = filepath.Join(nested, ".git", modulesDir)
	if err := os.MkdirAll(filepath.Join(modules, "deps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(modules, "mock"), filepath.Join(modules, "deps", "mock")); err != nil {
		t.Fatal(err)
	}
	renameSubmodule(t, nested, "deps/mock")
	tree, err := listHead(t, nested)
	spliced(t, tree, err, "mock/main.go")

	inside, _ := umbrella(t)
	if err := os.Symlink("mock", filepath.Join(inside, ".git", modulesDir, "alias")); err != nil {
		t.Fatal(err)
	}
	renameSubmodule(t, inside, "alias")
	tree, err = listHead(t, inside)
	spliced(t, tree, err, "mock/main.go")
}

func gitStdin(t *testing.T, dir, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = composetest.Env()
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func TestAGitlinkAtADotOrParentSegmentIsRejectedEvenWhenGitmodulesMapsItToAnInitializedSubmodule(t *testing.T) {
	dir, _ := umbrella(t)
	pin := gitT(t, dir, "rev-parse", "HEAD:mock")
	for _, at := range []string{"..", "."} {
		gitmodules := gitStdin(t, dir, "[submodule \"mock\"]\n\tpath = "+at+"\n\turl = ./mock\n", "hash-object", "-w", "--stdin")
		tree := gitStdin(t, dir, "100644 blob "+gitmodules+"\t.gitmodules\n160000 commit "+pin+"\t"+at+"\n", "mktree")
		_, err := Git{Dir: dir}.ListTree(context.Background(), gitT(t, dir, "commit-tree", tree, "-m", "gitlink at "+at))
		refusedAt(t, err, at, "not a clean relative path")
	}
	good := gitStdin(t, dir, "100644 blob "+gitT(t, dir, "rev-parse", "HEAD:.gitmodules")+"\t.gitmodules\n160000 commit "+pin+"\tmock\n", "mktree")
	listed, err := Git{Dir: dir}.ListTree(context.Background(), gitT(t, dir, "commit-tree", good, "-m", "canary"))
	spliced(t, listed, err, "mock/main.go")
}

func TestAnIncludeInACommittedGitmodulesIsNotFollowedAndItsEntriesDoNotMapAGitlink(t *testing.T) {
	dir, _ := umbrella(t)
	host := filepath.Join(t.TempDir(), "planted.cfg")
	writeFile(t, host, "[submodule \"planted\"]\n\tpath = mock\n")
	if err := os.Symlink("mock", filepath.Join(dir, ".git", modulesDir, "planted")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, gitmodulesPath), "[include]\n\tpath = "+host+"\n")
	gitT(t, dir, "commit", "-q", "-m", "include", "--", gitmodulesPath)
	if plain := gitT(t, dir, "config", "--blob", "HEAD:"+gitmodulesPath, "--list"); !strings.Contains(plain, "submodule.planted.path=mock") {
		t.Fatalf("canary: plain git config did not follow the include: %q", plain)
	}
	_, err := listHead(t, dir)
	refusedAt(t, err, "mock", "no submodule entry in .gitmodules")
}

func TestAnUninitializedSubmoduleOrAPinMissingFromItsModuleIsRejectedAtListingAndAnInitializedOneIsSpliced(t *testing.T) {
	dir, origin := umbrella(t)
	tree, err := listHead(t, dir)
	spliced(t, tree, err, "mock/main.go")
	if got, _ := tree.Blob(context.Background(), "mock/main.go"); string(got) != readFile(t, filepath.Join(origin, "main.go")) {
		t.Fatal("mock/main.go differs from the submodule's file")
	}

	missing, _ := umbrella(t)
	gitT(t, missing, "update-index", "--cacheinfo", "160000,"+gitT(t, missing, "rev-list", "--max-parents=0", "HEAD")+",mock")
	gitT(t, missing, "commit", "-q", "-m", "foreign pin")
	_, err = listHead(t, missing)
	refusedAt(t, err, "mock", "does not hold")

	uninit, _ := umbrella(t)
	composetest.Git(t, uninit, "submodule", "deinit", "-q", "-f", "mock")
	if err := os.RemoveAll(filepath.Join(uninit, ".git", modulesDir, "mock")); err != nil {
		t.Fatal(err)
	}
	_, err = listHead(t, uninit)
	refusedAt(t, err, "mock", "is not initialized")
}

func TestASubmoduleInitializedOnlyInALinkedWorktreeIsRejectedWithTheMainCheckoutHintUntilTheMainModuleHoldsThePin(t *testing.T) {
	dir, _ := umbrella(t)
	wt := filepath.Join(t.TempDir(), "wt")
	composetest.Git(t, dir, "worktree", "add", "-q", wt)
	composetest.Git(t, wt, "submodule", "update", "-q", "--init", "--recursive")
	writeFile(t, filepath.Join(wt, "mock", "extra.go"), "package main\n")
	composetest.Git(t, filepath.Join(wt, "mock"), "add", "-A")
	composetest.Git(t, filepath.Join(wt, "mock"), "commit", "-q", "-m", "linked only")
	composetest.Git(t, wt, "commit", "-q", "-a", "-m", "pin from the linked worktree")
	linked := filepath.Join(dir, ".git", "worktrees", "wt", modulesDir, "mock")
	pin := gitT(t, wt, "rev-parse", "HEAD:mock")
	if kind := gitT(t, linked, "cat-file", "-t", pin); kind != "commit" {
		t.Fatalf("canary: the linked worktree's module does not hold the pin: %s", kind)
	}
	_, err := listHead(t, wt)
	refusedAt(t, err, "mock", "does not read a linked worktree's submodules")
	composetest.Git(t, filepath.Join(dir, "mock"), "fetch", "-q", linked, "HEAD")
	tree, err := listHead(t, wt)
	spliced(t, tree, err, "mock/extra.go")
}

func TestSubmoduleObjectsAreReadAfterTheCheckoutIsRemovedWhilePlainGitInTheModuleFails(t *testing.T) {
	dir, origin := umbrella(t)
	pin := gitT(t, dir, "rev-parse", "HEAD:mock")
	if err := os.RemoveAll(filepath.Join(dir, "mock")); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".gitconfig"), "[safe]\n\tbareRepository = explicit\n")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	plain := exec.Command("git", "-C", filepath.Join(dir, ".git", modulesDir, "mock"), "ls-tree", pin)
	plain.Env = plainGitEnv()
	if out, err := plain.CombinedOutput(); err == nil || !strings.Contains(string(out), "cannot chdir") {
		t.Fatalf("canary: plain git in the module did not fail on the removed checkout: %v %s", err, out)
	}
	tree, err := listHead(t, dir)
	spliced(t, tree, err, "mock/main.go")
	if got, _ := tree.Blob(context.Background(), "mock/main.go"); string(got) != readFile(t, filepath.Join(origin, "main.go")) {
		t.Fatal("mock/main.go differs from the submodule's file")
	}
}

func pinLinks(t *testing.T, dir, pin string, from, to int) string {
	t.Helper()
	args := []string{"update-index", "--add"}
	var gitmodules strings.Builder
	for i := from; i < to; i++ {
		name := fmt.Sprintf("m%02d", i)
		args = append(args, "--cacheinfo", "160000,"+pin+",links/"+name)
		fmt.Fprintf(&gitmodules, "[submodule %q]\n\tpath = links/%s\n\turl = ./%s\n", name, name, name)
		if err := os.Symlink("mock", filepath.Join(dir, ".git", modulesDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(dir, gitmodulesPath), readFile(t, filepath.Join(dir, gitmodulesPath))+gitmodules.String())
	gitT(t, dir, args...)
	gitT(t, dir, "add", gitmodulesPath)
	gitT(t, dir, "commit", "-q", "-m", "links")
	return gitT(t, dir, "rev-parse", "HEAD")
}

func TestACommitPinningMoreThan64SubmodulesIsRejectedAtTheGitlinkThatCrossesTheCapAnd64Pass(t *testing.T) {
	dir, _ := umbrella(t)
	pin := gitT(t, dir, "rev-parse", "HEAD:mock")
	at64 := pinLinks(t, dir, pin, 0, maxSubmodules-1)
	tree, err := Git{Dir: dir}.ListTree(context.Background(), at64)
	if err != nil || len(tree.repos) != maxSubmodules+1 || !tree.Has("links/m62/main.go") {
		t.Fatalf("64 submodules: %d repositories, %v", len(tree.repos), err)
	}
	at65 := pinLinks(t, dir, pin, maxSubmodules-1, maxSubmodules)
	_, err = Git{Dir: dir}.ListTree(context.Background(), at65)
	refusedAt(t, err, "mock", "more than 64 submodules")
}

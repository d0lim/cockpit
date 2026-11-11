package compose

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose/composetest"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var dirtyMutations = []porttest.Mutation{
	{"each declared path is checked in the repository that holds it", "give every declared path to the root repository in dirtyPlan", "red: the plan sends a/src and a/b/f to the root", "TestDirtyPlanGivesEachDeclaredPathToTheDeepestRepositoryAndAddsEveryIncludedGitlinkToItsParent"},
	{"edits inside a submodule under a declared path are dirty", "give every declared path to the root repository in dirtyPlan", "red: nothing is listed for mock/main.go and mock/extra/", "TestAnEditOrUntrackedFileUnderADeclaredPathInsideASubmoduleIsDirtyAndAnEditOutsideItIsNot"},
	{"a submodule checked out at an unpinned commit is dirty whatever .gitmodules ignore= says", "drop --ignore-submodules=dirty from Git.status", "red: mock at a commit the umbrella does not record passes with ignore = all", "TestASubmoduleCheckoutAtAnUnpinnedCommitIsDirtyEvenWithIgnoreAllInGitmodules"},
	{"edits in a nested submodule are dirty", "skip submodules below the first level in Dirty", "red: mock/inner/lib.txt is not listed when the inner .gitmodules says ignore = all", "TestAnEditInANestedSubmoduleIsDirtyWhenTheInnerGitmodulesSaysIgnoreAll"},
	{"a submodule that is not checked out is clean", "treat an empty submodule directory as a foreign checkout", "red: the deinitialized submodule is listed", "TestADeinitializedSubmoduleWithAnEmptyDirectoryIsCleanAndANonEmptyNonCheckoutIsDirty"},
	{"a non-empty submodule directory that is not its own checkout is dirty", "skip a foreign submodule directory like an absent one in Dirty", "red: the planted file in the deinitialized submodule passes", "TestADeinitializedSubmoduleWithAnEmptyDirectoryIsCleanAndANonEmptyNonCheckoutIsDirty"},
}

func TestDirtyMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "dirty_test.go", dirtyMutations)
}

func TestDirtyPlanGivesEachDeclaredPathToTheDeepestRepositoryAndAddsEveryIncludedGitlinkToItsParent(t *testing.T) {
	tree := Tree{repos: map[string]repository{"": {}, "a": {prefix: "a"}, "a/b": {prefix: "a/b", parent: "a"}, "c": {prefix: "c"}}}
	step := func(prefix string, specs ...string) dirtyStep { return dirtyStep{prefix: prefix, pathspecs: specs} }
	cases := []struct {
		paths []string
		want  []dirtyStep
	}{
		{nil, []dirtyStep{step("", "."), step("a", "."), step("a/b", "."), step("c", ".")}},
		{[]string{"x", "y/z"}, []dirtyStep{step("", "x", "y/z")}},
		{[]string{"a/src"}, []dirtyStep{step("", "a"), step("a", "src")}},
		{[]string{"a/b/f", "x"}, []dirtyStep{step("", "a", "x"), step("a", "b"), step("a/b", "f")}},
		{[]string{"a"}, []dirtyStep{step("", "a"), step("a", "."), step("a/b", ".")}},
		{[]string{"c", "a/b"}, []dirtyStep{step("", "a", "c"), step("a", "b"), step("a/b", "."), step("c", ".")}},
		{[]string{"."}, []dirtyStep{step("", "."), step("a", "."), step("a/b", "."), step("c", ".")}},
		{[]string{"ab", "a-b/x"}, []dirtyStep{step("", "a-b/x", "ab")}},
	}
	evaluated := 0
	for _, c := range cases {
		if got := dirtyPlan(tree, c.paths); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: plan %+v, want %+v", c.paths, got, c.want)
		}
		evaluated++
	}
	if got := dirtyPlan(Tree{}, []string{"mock"}); !reflect.DeepEqual(got, []dirtyStep{step("", "mock")}) {
		t.Errorf("a tree without repositories: %+v", got)
	}
	if evaluated != len(cases) || evaluated == 0 {
		t.Fatalf("evaluated %d cases", evaluated)
	}
}

func dirtyFiles(t *testing.T, dir string, paths ...string) []string {
	t.Helper()
	g := Git{Dir: dir}
	tree, err := g.ListTree(context.Background(), gitT(t, dir, "rev-parse", "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	files, err := g.Dirty(context.Background(), tree, paths)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func expectDirty(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("dirty files %q, want %q", got, want)
	}
}

func TestAnEditOrUntrackedFileUnderADeclaredPathInsideASubmoduleIsDirtyAndAnEditOutsideItIsNot(t *testing.T) {
	dir, _ := umbrella(t)
	paths := []string{"mock/main.go", "mock/extra", "scripts"}
	expectDirty(t, dirtyFiles(t, dir, paths...))
	writeFile(t, filepath.Join(dir, "mock", "main.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "mock", "extra", "new.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "mock", "resp.go"), "package main\n")
	if plain := gitT(t, dir, "status", "--porcelain", "--untracked-files=normal", "--", "mock/main.go", "mock/extra"); plain != "" {
		t.Fatalf("canary: plain status at the top listed paths inside the submodule: %q", plain)
	}
	expectDirty(t, dirtyFiles(t, dir, paths...), "mock/extra/", "mock/main.go")
}

func TestASubmoduleCheckoutAtAnUnpinnedCommitIsDirtyEvenWithIgnoreAllInGitmodules(t *testing.T) {
	dir, _ := umbrella(t)
	gitT(t, dir, "config", "-f", gitmodulesPath, "submodule.mock.ignore", "all")
	gitT(t, dir, "commit", "-q", "-m", "ignore all", "--", gitmodulesPath)
	expectDirty(t, dirtyFiles(t, dir, "mock"))
	mock := filepath.Join(dir, "mock")
	writeFile(t, filepath.Join(mock, "extra.go"), "package main\n")
	composetest.Git(t, mock, "add", "-A")
	composetest.Git(t, mock, "commit", "-q", "-m", "unpinned")
	if plain := gitT(t, dir, "status", "--porcelain", "--", "mock"); plain != "" {
		t.Fatalf("canary: ignore = all did not hide the moved checkout from plain status: %q", plain)
	}
	expectDirty(t, dirtyFiles(t, dir, "mock"), "mock")
}

func TestAnEditInANestedSubmoduleIsDirtyWhenTheInnerGitmodulesSaysIgnoreAll(t *testing.T) {
	dir, _ := toyRepo(t, func(d string) { writeFile(t, filepath.Join(d, "mock", "inner", "lib.txt"), "inner\n") })
	composetest.Submodule(t, dir, "mock", "mock")
	mock := filepath.Join(dir, "mock")
	composetest.Submodule(t, mock, "inner", "inner")
	composetest.Git(t, mock, "config", "-f", gitmodulesPath, "submodule.inner.ignore", "all")
	composetest.Git(t, mock, "commit", "-q", "-m", "ignore all", "--", gitmodulesPath)
	commit(t, dir, func() {})
	expectDirty(t, dirtyFiles(t, dir, "mock"))
	writeFile(t, filepath.Join(mock, "inner", "lib.txt"), "edited\n")
	expectDirty(t, dirtyFiles(t, dir, "mock"), "mock/inner/lib.txt")
}

func TestADeinitializedSubmoduleWithAnEmptyDirectoryIsCleanAndANonEmptyNonCheckoutIsDirty(t *testing.T) {
	dir, _ := umbrella(t)
	writeFile(t, filepath.Join(dir, "mock", "main.go"), "package main\n")
	expectDirty(t, dirtyFiles(t, dir, "mock"), "mock/main.go")
	composetest.Git(t, dir, "submodule", "deinit", "-q", "-f", "mock")
	if entries, err := os.ReadDir(filepath.Join(dir, "mock")); err != nil || len(entries) != 0 {
		t.Fatalf("deinit left %d entries: %v", len(entries), err)
	}
	expectDirty(t, dirtyFiles(t, dir, "mock", "scripts"))
	writeFile(t, filepath.Join(dir, "mock", "planted.go"), "package main\n")
	expectDirty(t, dirtyFiles(t, dir, "mock", "scripts"), "mock")
}

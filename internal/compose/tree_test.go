package compose

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose/composetest"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var treeMutations = []porttest.Mutation{
	{"a gitlink no declared path references is neither resolved nor spliced", "splice every gitlink in Tree.splice whatever the paths", "red: each unreferenced unresolvable gitlink refuses the declaration", "TestAnUnresolvableGitlinkNoDeclaredPathReferencesIsAnEmptyDirectoryOfTheRunAndRefusesOnceADeclaredPathReferencesIt"},
	{"the .gitmodules of a repository is parsed only to splice one of its gitlinks", "parse the .gitmodules of every listed repository in Tree.add", "red: the unreadable .gitmodules refuses the declaration that references no gitlink", "TestAnUnresolvableGitlinkNoDeclaredPathReferencesIsAnEmptyDirectoryOfTheRunAndRefusesOnceADeclaredPathReferencesIt"},
	{"a referenced gitlink that does not resolve refuses the declaration", "record an unresolvable referenced gitlink as an empty repository in Tree.open", "red: each referenced unresolvable gitlink passes", "TestAnUnresolvableGitlinkNoDeclaredPathReferencesIsAnEmptyDirectoryOfTheRunAndRefusesOnceADeclaredPathReferencesIt"},
	{"a nested submodule is spliced only when a path reaches it", "splice every gitlink inside a spliced submodule in Tree.splice", "red: mock/inner/lib.txt is in the tree of the declaration that names only mock/main.go", "TestANestedSubmoduleIsSplicedWhenADeclaredPathReachesItAndIsAnEmptyDirectoryOtherwise"},
	{"a nested submodule a path reaches is spliced", "stop Tree.splice after the first round of gitlinks", "red: mock/inner/lib.txt is missing from the tree of the declaration that names mock", "TestANestedSubmoduleIsSplicedWhenADeclaredPathReachesItAndIsAnEmptyDirectoryOtherwise"},
	{"the declaration file reads through a submodule it lies in", "list the tree without the declaration path in ReadDeclaration", "red: the commit has no .cockpit/project.yaml", "TestADeclarationInsideASubmoduleIsReadFromItsPinnedCommit"},
}

func TestTreeMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "tree_test.go", treeMutations)
}

func vendored(t *testing.T) string {
	t.Helper()
	dir, _ := toyRepo(t, func(d string) { writeFile(t, filepath.Join(d, "vendor", "extra", "readme.txt"), "extra\n") })
	composetest.Submodule(t, dir, "vendor/extra", "extra")
	return dir
}

func readHead(t *testing.T, dir string) (Loaded, error) {
	t.Helper()
	return ReadDeclaration(context.Background(), Git{Dir: dir}, gitT(t, dir, "rev-parse", "HEAD"), "")
}

func reference(t *testing.T, dir, path string) {
	t.Helper()
	declare(t, dir, "dirty: [.cockpit,", "dirty: ["+path+", .cockpit,")
	gitT(t, dir, "commit", "-q", "-a", "-m", "reference "+path)
}

func runTreeEntries(t *testing.T, loaded Loaded, dir string) []os.DirEntry {
	t.Helper()
	l := testLayout(t)
	root, err := OpenRun(l.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := Extract(context.Background(), loaded.Tree, root); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(l.Src, filepath.FromSlash(dir)))
	if err != nil {
		t.Fatalf("src/%s: %v", dir, err)
	}
	return entries
}

func TestAnUnresolvableGitlinkNoDeclaredPathReferencesIsAnEmptyDirectoryOfTheRunAndRefusesOnceADeclaredPathReferencesIt(t *testing.T) {
	cases := []struct {
		name      string
		unresolve func(t *testing.T, dir string)
		field     string
		says      string
	}{
		{"an uninitialized submodule", func(t *testing.T, dir string) {
			composetest.Git(t, dir, "submodule", "deinit", "-q", "-f", "vendor/extra")
			if err := os.RemoveAll(filepath.Join(dir, ".git", modulesDir, "extra")); err != nil {
				t.Fatal(err)
			}
		}, "vendor/extra", submoduleHint},
		{"a gitlink without a .gitmodules entry", func(t *testing.T, dir string) {
			gitT(t, dir, "config", "-f", gitmodulesPath, "--remove-section", "submodule.extra")
			gitT(t, dir, "commit", "-q", "-m", "drop the entry", "--", gitmodulesPath)
		}, "vendor/extra", "no submodule entry in .gitmodules"},
		{"a .gitmodules that is not git config", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, gitmodulesPath), "[submodule\n")
			gitT(t, dir, "commit", "-q", "-m", "break .gitmodules", "--", gitmodulesPath)
		}, gitmodulesPath, "is not readable git config"},
	}
	evaluated := 0
	for _, c := range cases {
		dir := vendored(t)
		c.unresolve(t, dir)
		loaded, err := readHead(t, dir)
		if err != nil {
			t.Errorf("%s: the declaration that does not reference it was refused: %v", c.name, err)
			continue
		}
		if e := loaded.Tree.Entries["vendor/extra"]; e.Type != submoduleType || loaded.Tree.Has("vendor/extra/readme.txt") {
			t.Errorf("%s: vendor/extra is %+v and spliced %v", c.name, e, loaded.Tree.Has("vendor/extra/readme.txt"))
		}
		if entries := runTreeEntries(t, loaded, "vendor/extra"); len(entries) != 0 {
			t.Errorf("%s: src/vendor/extra holds %d entries, want an empty directory", c.name, len(entries))
		}
		reference(t, dir, "vendor/extra")
		_, err = readHead(t, dir)
		if r := rejection(t, err); r.Kind != RejectSubmodule || r.Field != c.field || !strings.Contains(r.Message, c.says) {
			t.Errorf("%s: referenced: %v, want a submodule rejection at %s saying %q", c.name, r, c.field, c.says)
		}
		evaluated++
	}
	if evaluated != len(cases) || evaluated != 3 {
		t.Fatalf("evaluated %d of 3 cases", evaluated)
	}
}

func TestANestedSubmoduleIsSplicedWhenADeclaredPathReachesItAndIsAnEmptyDirectoryOtherwise(t *testing.T) {
	dir, _ := toyRepo(t, func(d string) { writeFile(t, filepath.Join(d, "mock", "inner", "lib.txt"), "inner\n") })
	composetest.Submodule(t, dir, "mock", "mock")
	composetest.Submodule(t, filepath.Join(dir, "mock"), "inner", "inner")
	commit(t, dir, func() {})
	wide, err := readHead(t, dir)
	if err != nil || !wide.Tree.Has("mock/inner/lib.txt") {
		t.Fatalf("the declaration that names mock: inner spliced %v, %v", wide.Tree.Has("mock/inner/lib.txt"), err)
	}
	if got, err := wide.Tree.Blob(context.Background(), "mock/inner/lib.txt"); err != nil || string(got) != "inner\n" {
		t.Fatalf("mock/inner/lib.txt reads %q, %v", got, err)
	}
	declare(t, dir, "fixtures, mock, scripts", "fixtures, mock/main.go, scripts")
	declare(t, dir, "inputs: [mock]", "inputs: [mock/main.go]")
	gitT(t, dir, "commit", "-q", "-a", "-m", "name only mock/main.go")
	narrow, err := readHead(t, dir)
	if err != nil || !narrow.Tree.Has("mock/main.go") || narrow.Tree.Has("mock/inner/lib.txt") {
		t.Fatalf("the declaration that names only mock/main.go: mock spliced %v, inner spliced %v, %v", narrow.Tree.Has("mock/main.go"), narrow.Tree.Has("mock/inner/lib.txt"), err)
	}
	if entries := runTreeEntries(t, narrow, "mock/inner"); len(entries) != 0 {
		t.Fatalf("src/mock/inner holds %d entries, want an empty directory", len(entries))
	}
}

func TestADeclarationInsideASubmoduleIsReadFromItsPinnedCommit(t *testing.T) {
	dir, _ := toyRepo(t, nil)
	composetest.Submodule(t, dir, ".cockpit", "declarations")
	sha := gitT(t, dir, "rev-parse", "HEAD")
	if bare, err := (Git{Dir: dir}).ListTree(context.Background(), sha); err != nil || bare.Has(DeclarationPath) || bare.Entries[".cockpit"].Type != submoduleType {
		t.Fatalf("canary: a listing without paths holds %s %v, %v", DeclarationPath, bare.Has(DeclarationPath), err)
	}
	loaded, err := ReadDeclaration(context.Background(), Git{Dir: dir}, sha, "")
	if err != nil || loaded.Declaration.Project.Builder != "builder" {
		t.Fatalf("the declaration in the submodule: builder %q, %v", loaded.Declaration.Project.Builder, err)
	}
}

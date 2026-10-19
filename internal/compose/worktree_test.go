package compose

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var worktreeMutations = []porttest.Mutation{
	{"the extracted tree holds exactly the commit's files", "drop the file count comparison in Extract", "red: an extraction into a directory that already holds a stray file passes", "TestExtractWritesEveryCommitFileIntoSrcAndBuildAndRefusesAStrayTree"},
	{"run directory writes cannot follow a committed link out of the run", "write with os.WriteFile on joined paths instead of the os.Root", "red: a write through src/escape lands outside the run directory", "TestWritesThroughACommittedLinkCannotLeaveTheRunDirectory"},
	{"runner sources never carry links into the work tree", "copy symlinks instead of refusing them", "red: a runner source with a link is copied", "TestWorkTreeRefusesALinkInsideARunnerSourceAndCopiesCleanSources"},
	{"the private env in the work tree is 600 in a 700 directory", "write the private env with os.WriteFile 0644", "red: work/e2e/e2e.private.env.json is 644", "TestWorkTreeHoldsScriptsDeclarationSourcesAndAPrivateEnvAt600"},
}

func TestWorktreeMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "worktree_test.go", worktreeMutations)
}

func extracted(t *testing.T, mutate func(dir string)) (Layout, *os.Root, Loaded) {
	t.Helper()
	dir, sha := toyRepo(t, mutate)
	g := Git{Dir: dir}
	loaded, err := ReadDeclaration(context.Background(), g, sha)
	if err != nil {
		t.Fatal(err)
	}
	l := testLayout(t)
	root, err := OpenRun(l.Root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	if err := Extract(context.Background(), g, loaded.Tree, root); err != nil {
		t.Fatal(err)
	}
	return l, root, loaded
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestExtractWritesEveryCommitFileIntoSrcAndBuildAndRefusesAStrayTree(t *testing.T) {
	l, _, loaded := extracted(t, nil)
	want := loaded.Tree.Files()
	for _, dir := range []string{l.Src, l.Build} {
		if got := countFiles(t, dir); got != want || want == 0 {
			t.Errorf("%s holds %d files, commit has %d", dir, got, want)
		}
	}
	if readFile(t, filepath.Join(l.Src, DeclarationPath)) != readFile(t, filepath.Join(toyDir(t), DeclarationPath)) {
		t.Fatal("the extracted declaration differs from the committed one")
	}
	if mode(t, filepath.Join(l.Src, "make-env.sh"))&0o100 == 0 {
		t.Fatal("the executable bit of make-env.sh was lost")
	}
	dir, sha := toyRepo(t, nil)
	g := Git{Dir: dir}
	tree, err := g.ListTree(context.Background(), sha)
	if err != nil {
		t.Fatal(err)
	}
	tree.Entries["phantom.txt"] = Entry{Mode: "100644", Type: "blob", ID: sha}
	l2 := testLayout(t)
	root2, err := OpenRun(l2.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer root2.Close()
	if err := Extract(context.Background(), g, tree, root2); err == nil || !strings.Contains(err.Error(), "extracted") {
		t.Fatalf("a count mismatch passed: %v", err)
	}
}

func TestWritesThroughACommittedLinkCannotLeaveTheRunDirectory(t *testing.T) {
	outside := t.TempDir()
	_, root, _ := extracted(t, func(d string) {
		if err := os.Symlink(outside, filepath.Join(d, "escape")); err != nil {
			t.Fatal(err)
		}
	})
	if err := writePrivate(root, "src/escape/planted.txt", []byte(planted)); err == nil {
		t.Fatal("a write through the committed link succeeded")
	}
	if exists(filepath.Join(outside, "planted.txt")) {
		t.Fatal("the planted file landed outside the run directory")
	}
	if err := writePrivate(root, "src/inside.txt", []byte("ok")); err != nil {
		t.Fatalf("a write inside the run directory failed: %v", err)
	}
}

func TestWorkTreeRefusesALinkInsideARunnerSourceAndCopiesCleanSources(t *testing.T) {
	_, root, loaded := extracted(t, func(d string) {
		if err := os.Symlink("/etc/hosts", filepath.Join(d, "fixtures", "hosts")); err != nil {
			t.Fatal(err)
		}
	})
	err := MakeWorkTree(root, loaded.Declaration)
	if err == nil || !strings.Contains(err.Error(), "symbolic link") || !strings.Contains(err.Error(), "fixtures/hosts") {
		t.Fatalf("a runner source with a link: %v", err)
	}
	cl, clean, cleanLoaded := extracted(t, nil)
	writeFile(t, filepath.Join(cl.Src, PrivateEnvName), "{}")
	if err := MakeWorkTree(clean, cleanLoaded.Declaration); err != nil {
		t.Fatalf("clean sources were refused: %v", err)
	}
}

func TestWorkTreeHoldsScriptsDeclarationSourcesAndAPrivateEnvAt600(t *testing.T) {
	l, root, loaded := extracted(t, nil)
	loaded.Declaration.Env.Outputs = loaded.Declaration.Env.Outputs[:1]
	if err := MakeWorkTree(root, loaded.Declaration); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"scripts/suites/smoke.ts", "scripts/env.json", ".cockpit/project.yaml", "source/fixtures/seed.json"} {
		if !exists(filepath.Join(l.Work, p)) {
			t.Errorf("work/%s is missing", p)
		}
	}
	if exists(filepath.Join(l.Work, "mock")) || exists(filepath.Join(l.Work, "source", "fixtures", "fixtures")) {
		t.Fatal("the work tree holds paths it should not")
	}
	priv := filepath.Join(l.Work, "e2e", PrivateEnvName)
	if mode(t, priv) != 0o600 || mode(t, filepath.Join(l.Work, "e2e")) != 0o700 || mode(t, l.Work) != 0o700 {
		t.Fatalf("modes: priv %v e2e %v work %v", mode(t, priv), mode(t, filepath.Join(l.Work, "e2e")), mode(t, l.Work))
	}
	if !strings.Contains(readFile(t, priv), `"local"`) {
		t.Fatalf("empty private env %q", readFile(t, priv))
	}
	l2, root2, loaded2 := extracted(t, nil)
	writeFile(t, filepath.Join(l2.Src, PrivateEnvName), `{"local":{"appToken":"`+planted+`"}}`)
	if err := MakeWorkTree(root2, loaded2.Declaration); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(l2.Work, "e2e", PrivateEnvName)); !strings.Contains(got, planted) {
		t.Fatalf("the generated private env was not carried into the work tree: %q", got)
	}
	_, root3, loaded3 := extracted(t, nil)
	if err := MakeWorkTree(root3, loaded3.Declaration); err == nil {
		t.Fatal("a declared private env output that was never generated passed")
	}
}

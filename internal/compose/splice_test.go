package compose

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose/composetest"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var spliceMutations = []porttest.Mutation{
	{"the run commit holds every file of every pinned submodule commit, recursively", "splice only the first level of submodules", "red: mock/inner/lib.txt is missing and the count differs from ls-files --recurse-submodules", "TestSplicedTreeCountsTheSameFilesAsLsFilesRecurseSubmodulesAndReadsBlobsFromEachRepository"},
	{"a path claimed by two submodule names is refused", "keep the last name for a path claimed twice in parseSubmoduleNames", "red: lib claimed by a and b parses", "TestParseSubmoduleNamesKeepsDottedAndSlashedNamesAndRejectsAPathClaimedTwice"},
}

func TestSpliceMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "splice_test.go", spliceMutations)
}

func TestSplicedTreeCountsTheSameFilesAsLsFilesRecurseSubmodulesAndReadsBlobsFromEachRepository(t *testing.T) {
	dir, _ := toyRepo(t, func(d string) { writeFile(t, filepath.Join(d, "mock", "inner", "lib.txt"), "inner\n") })
	composetest.Submodule(t, dir, "mock", "mock")
	composetest.Submodule(t, filepath.Join(dir, "mock"), "inner", "inner")
	sha := commit(t, dir, func() {})
	tree, err := Git{Dir: dir}.ListTree(context.Background(), sha)
	if err != nil {
		t.Fatal(err)
	}
	files := strings.Fields(gitT(t, dir, "ls-files", "--recurse-submodules"))
	if tree.Files() != len(files) || !slices.Contains(files, "mock/inner/lib.txt") {
		t.Fatalf("tree counts %d files, ls-files --recurse-submodules %d", tree.Files(), len(files))
	}
	for _, f := range files {
		if !tree.Has(f) || tree.IsDir(f) {
			t.Errorf("%s is not a file of the spliced tree", f)
		}
	}
	for _, p := range []string{DeclarationPath, "mock/main.go", "mock/inner/lib.txt"} {
		got, err := tree.Blob(context.Background(), p)
		if err != nil || string(got) != readFile(t, filepath.Join(dir, p)) {
			t.Errorf("%s reads %q, %v", p, got, err)
		}
	}
	for path, pin := range map[string]string{"mock": gitT(t, dir, "rev-parse", "HEAD:mock"), "mock/inner": gitT(t, filepath.Join(dir, "mock"), "rev-parse", "HEAD:inner")} {
		if e := tree.Entries[path]; !tree.IsDir(path) || e.Type != submoduleType || e.ID != pin || e.repo != path {
			t.Errorf("%s is %+v, want a directory pinned at %s", path, e, pin)
		}
	}
}

func TestParseSubmoduleNamesKeepsDottedAndSlashedNamesAndRejectsAPathClaimedTwice(t *testing.T) {
	listing := "submodule.dotted.name.path\nlib/a\x00submodule.deps/inner.path\nlib/b\x00submodule.deps/inner.url\n../x\x00" +
		"submodule.path\nlib/c\x00submodule..path\nlib/d\x00core.bare\nfalse\x00submodule.flag.path\x00submodule.same.path\nlib/e\x00submodule.same.path\nlib/e\x00"
	names, err := parseSubmoduleNames([]byte(listing))
	want := map[string]string{"lib/a": "dotted.name", "lib/b": "deps/inner", "lib/e": "same"}
	if err != nil || !maps.Equal(names, want) {
		t.Fatalf("names %v, %v", names, err)
	}
	_, err = parseSubmoduleNames([]byte("submodule.a.path\nlib\x00submodule.b.path\nlib\x00"))
	if err == nil || !strings.Contains(err.Error(), `"a"`) || !strings.Contains(err.Error(), `"b"`) {
		t.Fatalf("a path claimed twice: %v", err)
	}
}

package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose/composetest"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var cleanFirstMutations = []porttest.Mutation{
	{"an unclean declared path is refused as confinement before any gitlink it names is spliced", "drop the cleanDeclaredPaths call before Tree.splice in ReadDeclaration", "red: vendor/extra/..\\..\\etc is refused as submodule with the main checkout hint", "TestAnUncleanDeclaredPathThroughAnUninitializedSubmoduleIsRefusedAsConfinementWhileACleanPathIntoItIsRefusedAsSubmodule"},
}

func TestCleanFirstMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "cleanfirst_test.go", cleanFirstMutations)
}

func uninitialized(t *testing.T) string {
	t.Helper()
	dir := vendored(t)
	composetest.Git(t, dir, "submodule", "deinit", "-q", "-f", "vendor/extra")
	if err := os.RemoveAll(filepath.Join(dir, ".git", modulesDir, "extra")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAnUncleanDeclaredPathThroughAnUninitializedSubmoduleIsRefusedAsConfinementWhileACleanPathIntoItIsRefusedAsSubmodule(t *testing.T) {
	cases := []struct {
		path string
		kind RejectKind
		says string
	}{
		{`vendor/extra/..\..\etc`, RejectConfinement, "must be a clean relative path"},
		{`vendor/extra/a\b`, RejectConfinement, "must be a clean relative path"},
		{"vendor/extra/readme.txt", RejectSubmodule, submoduleHint},
		{"vendor/extra", RejectSubmodule, submoduleHint},
	}
	evaluated := 0
	for _, c := range cases {
		dir := uninitialized(t)
		reference(t, dir, c.path)
		_, err := readHead(t, dir)
		if r := rejection(t, err); r.Kind != c.kind || !strings.Contains(r.Message, c.says) {
			t.Errorf("%s: %v, want %s saying %q", c.path, r, c.kind, c.says)
		}
		evaluated++
	}
	if evaluated != 4 {
		t.Fatalf("evaluated %d of 4 cases", evaluated)
	}
	clean := vendored(t)
	reference(t, clean, "vendor/extra/readme.txt")
	if loaded, err := readHead(t, clean); err != nil || !loaded.Tree.Has("vendor/extra/readme.txt") {
		t.Fatalf("a clean path into an initialized submodule: spliced %v, %v", loaded.Tree.Has("vendor/extra/readme.txt"), err)
	}
}

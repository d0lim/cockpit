package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/compose/composetest"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var gitlinkMutations = []porttest.Mutation{
	{"a submit passes when the only uninitialized submodule is one no declared path references", "splice every gitlink in compose Tree.splice whatever the paths", "red: the umbrella is refused as an uninitialized submodule at vendor/extra", "TestAnUmbrellaWhoseUninitializedSubmoduleNoDeclaredPathReferencesRunsWithAnEmptyDirectoryThereUntilTheDeclarationReferencesIt"},
}

func TestGitlinkMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "gitlink_test.go", gitlinkMutations)
}

func TestAnUmbrellaWhoseUninitializedSubmoduleNoDeclaredPathReferencesRunsWithAnEmptyDirectoryThereUntilTheDeclarationReferencesIt(t *testing.T) {
	w := newWorld(t, nil, func(d string) {
		if err := os.MkdirAll(filepath.Join(d, "vendor", "extra"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "vendor", "extra", "readme.txt"), []byte("extra\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	composetest.Submodule(t, w.repo, "vendor/extra", "extra")
	composetest.Git(t, w.repo, "submodule", "deinit", "-q", "-f", "vendor/extra")
	if err := os.RemoveAll(filepath.Join(w.repo, ".git", "modules", "extra")); err != nil {
		t.Fatal(err)
	}
	m := w.submit()
	dir, _ := w.hub.Store().RunDir(m.ID)
	entries, err := os.ReadDir(filepath.Join(dir, "src", "vendor", "extra"))
	if err != nil || len(entries) != 0 || m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("src/vendor/extra: %d entries, %v, verdict %q", len(entries), err, m.Verdict)
	}
	replaceIn(t, filepath.Join(w.repo, compose.DeclarationPath), "dirty: [.cockpit,", "dirty: [vendor/extra, .cockpit,")
	gitIn(t, w.repo, "commit", "-q", "-a", "-m", "reference vendor/extra")
	if f := refused(t, w, w.repo); f == nil || f.Code != ticker.CodeRejectedNoHarness || !strings.Contains(f.Reason, "declaration rejected (submodule) at vendor/extra") || !strings.Contains(f.Reason, "not initialized") {
		t.Fatalf("the declaration that references the uninitialized submodule: %+v", f)
	}
}

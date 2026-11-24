package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/compose/composetest"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var gitlinkMutations = []porttest.Mutation{
	{"a submit passes when the only uninitialized submodule is one no declared path references", "splice every gitlink in compose Tree.splice whatever the paths", "red: the umbrella is refused as an uninitialized submodule at vendor/extra", "TestAnUmbrellaWhoseUninitializedSubmoduleNoDeclaredPathReferencesRunsWithAnEmptyDirectoryThereUntilTheDeclarationReferencesIt"},
	{"the static seal judges binds against the run's tree, so a bind into a submodule the run leaves out fails before any build", "leave Tree out of the seal.Input that stages Env.SealStatic builds", "red: the run whose db binds src/vendor/extra passes", "TestABindIntoAnInitializedSubmoduleNoDeclaredPathReferencesFailsTheStaticSealBeforeAnyBuildAndRunsOnceReferenced"},
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

func bindingFixture(t *testing.T, service, source string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "seal", "testdata", "config.toy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	svc := doc["services"].(map[string]any)[service].(map[string]any)
	volumes, _ := svc["volumes"].([]any)
	svc["volumes"] = append(volumes, map[string]any{"type": "bind", "source": source, "target": "/mnt/extra"})
	out, _ := json.Marshal(doc)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestABindIntoAnInitializedSubmoduleNoDeclaredPathReferencesFailsTheStaticSealBeforeAnyBuildAndRunsOnceReferenced(t *testing.T) {
	fixture := bindingFixture(t, "db", "/fixture/runs/r-toy/src/vendor/extra")
	w := newWorld(t, func(f *fakedocker.Fake) { f.ConfigFixture = fixture }, func(d string) {
		if err := os.MkdirAll(filepath.Join(d, "vendor", "extra"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "vendor", "extra", "readme.txt"), []byte("extra\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	composetest.Submodule(t, w.repo, "vendor/extra", "extra")
	m := w.submit()
	closedState(t, m, lifecycle.VerdictFailed, lifecycle.EnvAbsent, lifecycle.Preparing)
	if !strings.Contains(m.Notice, "static seal violated: mounts.binds") || !strings.Contains(m.Notice, "the submodule vendor/extra, which no declared path references") {
		t.Fatalf("notice %q", m.Notice)
	}
	if indexOf(w.verbs(), "compose build") >= 0 || indexOf(w.verbs(), "compose run builder") >= 0 {
		t.Fatalf("the build ran after the static seal failed: %v", w.verbs())
	}
	replaceIn(t, filepath.Join(w.repo, compose.DeclarationPath), "dirty: [.cockpit,", "dirty: [vendor/extra, .cockpit,")
	gitIn(t, w.repo, "commit", "-q", "-a", "-m", "reference vendor/extra")
	if m := w.submit(); m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("the run that references vendor/extra: verdict %q, notice %q", m.Verdict, m.Notice)
	}
}

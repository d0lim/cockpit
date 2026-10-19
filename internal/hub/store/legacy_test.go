package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var legacyMutations = []porttest.Mutation{
	{"an old state.json refuses the start", "drop the schemaVersion check", "red: a home with only the old state.json passes", "TestHomeWithThePreviousLayoutIsRefusedWithGuidance"},
	{"old run meta refuses the start", "drop the harness field check", "red: a home with only old run meta passes", "TestHomeWithThePreviousLayoutIsRefusedWithGuidance"},
	{"the current layout is accepted", "always return ErrLegacyLayout", "red: the current layout or an empty home is refused", "TestEmptyHomeAndTheCurrentLayoutAreAccepted"},
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHomeWithThePreviousLayoutIsRefusedWithGuidance(t *testing.T) {
	full := t.TempDir()
	copyTree(t, filepath.Join("..", "testdata", "legacy-home"), full)
	onlyState := t.TempDir()
	copyTree(t, filepath.Join("..", "testdata", "legacy-home"), onlyState)
	os.RemoveAll(filepath.Join(onlyState, "runs"))
	onlyMeta := t.TempDir()
	copyTree(t, filepath.Join("..", "testdata", "legacy-home"), onlyMeta)
	os.Remove(filepath.Join(onlyMeta, "state.json"))
	cases := map[string]string{full: "harness field", onlyState: "schemaVersion 1", onlyMeta: "1 run meta files"}
	for home, evidence := range cases {
		err := RefuseLegacyLayout(home)
		if !errors.Is(err, ErrLegacyLayout) {
			t.Fatalf("%s: err %v", home, err)
		}
		for _, want := range []string{evidence, home, "point COCKPIT_HOME at an empty directory"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("%q lacks %q", err, want)
			}
		}
	}
	if len(cases) != 3 {
		t.Fatal("case count")
	}
}

func TestEmptyHomeAndTheCurrentLayoutAreAccepted(t *testing.T) {
	if err := RefuseLegacyLayout(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("missing home refused: %v", err)
	}
	s := Open(homeFixture(t))
	mustWriteMeta(t, s, fixture(runA))
	if err := s.WriteState(EmptyState()); err != nil {
		t.Fatal(err)
	}
	if err := RefuseLegacyLayout(s.Home()); err != nil {
		t.Fatalf("current layout refused: %v", err)
	}
	if err := RefuseLegacyLayout(filepath.Join("..", "testdata", "legacy-home")); err == nil {
		t.Fatal("canary: the legacy testdata was accepted")
	}
}

func TestLegacyMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "legacy_test.go", legacyMutations)
}

package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var suitesMutations = []porttest.Mutation{
	{"the runner bundles the suite from the directory COCKPIT_SUITES names", "keep ./suites/ in driverSource whatever COCKPIT_SUITES holds", "red: the suite under scripts/cases is not found and the run exits 2", "TestTheRunnerBundlesTheSuiteFromCOCKPITSUITESAndRefusesADirectoryOutsideTheScripts"},
	{"a suites directory outside the scripts directory is refused", "drop the IsLocal check on the suites directory in Main", "red: COCKPIT_SUITES outside the scripts directory bundles and writes a manifest", "TestTheRunnerBundlesTheSuiteFromCOCKPITSUITESAndRefusesADirectoryOutsideTheScripts"},
	{"the driver imports the suite through an encoded string literal", "write the import path between plain quotes in driverSource", "red: the suite under a directory named with a quote does not bundle", "TestASuitesDirectoryWhoseNameHoldsAQuoteOrABackslashStillBundles"},
}

func TestSuitesMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "suites_test.go", suitesMutations)
}

func TestTheRunnerBundlesTheSuiteFromCOCKPITSUITESAndRefusesADirectoryOutsideTheScripts(t *testing.T) {
	w := newWorld(t, core, nil)
	cases := filepath.Join(w.scripts, "cases")
	if err := os.MkdirAll(cases, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(w.scripts, "suites", "fake.ts"), filepath.Join(cases, "fake.ts")); err != nil {
		t.Fatal(err)
	}
	if got := w.run(nil); got.code != 2 || w.exists("manifest.json") {
		t.Fatalf("canary: without COCKPIT_SUITES the suite under cases ran: exit %d %v", got.code, got.stderr)
	}
	if got := w.run(map[string]string{"COCKPIT_SUITES": cases}); got.code != 0 || !w.exists("manifest.json") {
		t.Fatalf("COCKPIT_SUITES %s: exit %d %v", cases, got.code, got.stderr)
	}
	os.Remove(filepath.Join(w.dir, "manifest.json"))
	outside := filepath.Join(w.root, "cases")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(cases, "fake.ts"), filepath.Join(outside, "fake.ts")); err != nil {
		t.Fatal(err)
	}
	got := w.run(map[string]string{"COCKPIT_SUITES": outside})
	if got.code != 2 || w.exists("manifest.json") || !strings.Contains(strings.Join(got.stderr, "\n"), "COCKPIT_SUITES") {
		t.Fatalf("COCKPIT_SUITES outside the scripts directory: exit %d %v", got.code, got.stderr)
	}
}

func TestASuitesDirectoryWhoseNameHoldsAQuoteOrABackslashStillBundles(t *testing.T) {
	ran := 0
	for _, name := range []string{`cases"large`, `cases\large`} {
		w := newWorld(t, core, nil)
		dir := filepath.Join(w.scripts, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(w.scripts, "suites", "fake.ts"), filepath.Join(dir, "fake.ts")); err != nil {
			t.Fatal(err)
		}
		if got := w.run(map[string]string{"COCKPIT_SUITES": dir}); got.code != 0 || !w.exists("manifest.json") {
			t.Errorf("COCKPIT_SUITES %s: exit %d %v", dir, got.code, got.stderr)
			continue
		}
		ran++
	}
	if ran != 2 {
		t.Fatalf("%d of 2 directories bundled", ran)
	}
}

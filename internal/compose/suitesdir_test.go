package compose

import (
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var suitesDirMutations = []porttest.Mutation{
	{"the runner reads suites from the declared suites directory", "never set COCKPIT_SUITES in the runner environment of Harness", "red: a declaration with scripts.suites scripts/alt gives the runner no COCKPIT_SUITES", "TestTheRunnerGetsCOCKPITSUITESOnlyWhenTheSuitesDirectoryIsNotSuitesUnderTheScriptsDirectory"},
	{"a declaration that keeps suites under scripts.dir/suites keeps the harness as it was", "set COCKPIT_SUITES whatever scripts.suites holds", "red: the toy harness carries COCKPIT_SUITES", "TestTheRunnerGetsCOCKPITSUITESOnlyWhenTheSuitesDirectoryIsNotSuitesUnderTheScriptsDirectory"},
}

func TestSuitesDirMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "suitesdir_test.go", suitesDirMutations)
}

func TestTheRunnerGetsCOCKPITSUITESOnlyWhenTheSuitesDirectoryIsNotSuitesUnderTheScriptsDirectory(t *testing.T) {
	doc, _, in := harnessDoc(t, nil)
	env := service(t, doc, RoleRunner)["environment"].(map[string]any)
	if _, set := env["COCKPIT_SUITES"]; set || env["COCKPIT_RUNNER_IMAGE"] != in.RunnerImage {
		t.Fatalf("runner environment of the toy declaration: %v", env)
	}
	for value, want := range map[contract.RelPath]string{"scripts/alt": "/work/scripts/alt", "scripts/cases/large": "/work/scripts/cases/large"} {
		in.Loaded.Declaration.Scripts.Suites = value
		doc, _, _ := harnessDocFrom(t, in)
		env := service(t, doc, RoleRunner)["environment"].(map[string]any)
		if env["COCKPIT_SUITES"] != want || env["COCKPIT_RUNNER_IMAGE"] != in.RunnerImage || len(env) != 4 {
			t.Errorf("scripts.suites %s: runner environment %v, want COCKPIT_SUITES %s", value, env, want)
		}
	}
}

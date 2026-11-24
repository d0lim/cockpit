package compose

import (
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var scriptsEnvMutations = []porttest.Mutation{
	{"scripts.env stays inside scripts.dir", "drop scripts.env from the scripts.dir prefix loop in confine", "red: scripts.env fixtures/seed.json passes", "TestScriptsEnvOutsideTheScriptsDirectoryOrNotAFileIsRejectedAndAFileInsideIsAccepted"},
	{"scripts.env names a file of the commit", "add scripts.env to declaredPaths with mustFile false", "red: scripts.env scripts/suites and scripts/missing.json pass", "TestScriptsEnvOutsideTheScriptsDirectoryOrNotAFileIsRejectedAndAFileInsideIsAccepted"},
	{"the runner reads another public env only when the declaration names one", "set E2E_ENV in the runner environment of Harness whatever scripts.env holds", "red: a harness without scripts.env carries E2E_ENV", "TestTheRunnerGetsE2EEnvAtItsPathUnderWorkScriptsOnlyWhenScriptsEnvIsSet"},
}

func TestScriptsEnvMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "scriptsenv_test.go", scriptsEnvMutations)
}

func TestScriptsEnvOutsideTheScriptsDirectoryOrNotAFileIsRejectedAndAFileInsideIsAccepted(t *testing.T) {
	tree := mustLoadToy(t).Tree
	pairs := []struct {
		refused contract.RelPath
		says    string
		allowed contract.RelPath
	}{
		{"fixtures/seed.json", "inside scripts.dir", "scripts/env.json"},
		{"scripts/suites", "not a file", "scripts/suites/smoke.ts"},
		{"scripts/missing.json", "not a file", "scripts/env.json"},
		{"../env.json", "clean relative path", "scripts/env.json"},
		{"scripts/../env.json", "clean relative path", "scripts/flows/health.ts"},
	}
	evaluated := 0
	for _, p := range pairs {
		d := toyDeclaration(t)
		d.Scripts.Env = p.allowed
		if err := confine(d, tree); err != nil {
			t.Errorf("scripts.env %q was rejected: %v", p.allowed, err)
		}
		d.Scripts.Env = p.refused
		r := rejection(t, confine(d, tree))
		if r.Kind != RejectConfinement || r.Field != "scripts.env" || !strings.Contains(r.Message, p.says) {
			t.Errorf("scripts.env %q: %v, want confinement at scripts.env saying %q", p.refused, r, p.says)
		}
		evaluated++
	}
	if evaluated != 5 {
		t.Fatalf("evaluated %d pairs", evaluated)
	}
}

func TestTheRunnerGetsE2EEnvAtItsPathUnderWorkScriptsOnlyWhenScriptsEnvIsSet(t *testing.T) {
	doc, _, in := harnessDoc(t, nil)
	env := service(t, doc, RoleRunner)["environment"].(map[string]any)
	if _, set := env["E2E_ENV"]; set || env["COCKPIT_RUNNER_IMAGE"] != in.RunnerImage {
		t.Fatalf("runner environment without scripts.env: %v", env)
	}
	for value, want := range map[contract.RelPath]string{"scripts/env.alt.json": "/work/scripts/env.alt.json", "scripts/envs/large.json": "/work/scripts/envs/large.json"} {
		in.Loaded.Declaration.Scripts.Env = value
		doc, _, _ := harnessDocFrom(t, in)
		env := service(t, doc, RoleRunner)["environment"].(map[string]any)
		if env["E2E_ENV"] != want || env["COCKPIT_RUNNER_IMAGE"] != in.RunnerImage || len(env) != 4 {
			t.Errorf("scripts.env %s: runner environment %v, want E2E_ENV %s", value, env, want)
		}
	}
}

package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/headless"
)

func TestRunWithoutAProjectOrWithAnUnknownFlagPrintsItsUsageAndExits3(t *testing.T) {
	for _, args := range [][]string{{"run"}, {"run", "--project"}, {"run", "--project", ".", "--unsealed"}} {
		code, out, errOut := call(nil, args...)
		if code != 3 || out != "" || !strings.Contains(errOut, "usage: cockpit run --project <path>") {
			t.Errorf("%v: exit %d stdout %q stderr %q", args, code, out, errOut)
		}
	}
	if code, _, errOut := call(nil, "fly"); code != 2 || !strings.Contains(errOut, "run --project") {
		t.Fatalf("the top-level usage does not list run: %q", errOut)
	}
}

func TestRunFlagsReachTheRunOptionsAndTheStateDirectoryAndSubmitterReachItsDeps(t *testing.T) {
	previous := headlessRun
	t.Cleanup(func() { headlessRun = previous })
	var got []headless.Options
	var deps []headless.Deps
	headlessRun = func(_ context.Context, opts headless.Options, d headless.Deps) int {
		got, deps = append(got, opts), append(deps, d)
		return 7
	}
	home, project := t.TempDir(), t.TempDir()
	out := filepath.Join(project, "bundle")
	cases := []struct {
		args      []string
		env       map[string]string
		want      headless.Options
		submitter string
	}{
		{[]string{"run", "--project", project}, map[string]string{"COCKPIT_HOME": home}, headless.Options{Project: project}, "cli"},
		{[]string{"run", "--project", project, "--suite", "smoke", "--unseal-check", "--keep"}, map[string]string{"COCKPIT_HOME": home, "COCKPIT_SUBMITTER": "ci"}, headless.Options{Project: project, Suite: "smoke", UnsealCheck: true, Keep: true}, "ci"},
		{[]string{"run", "--keep", "--project", filepath.Join(project, "."), "--out", out, "--archive", out + ".tar.gz"}, map[string]string{"COCKPIT_HOME": home}, headless.Options{Project: project, Keep: true, Out: out, Archive: out + ".tar.gz"}, "cli"},
	}
	if len(cases) != 3 {
		t.Fatalf("%d cases", len(cases))
	}
	for i, c := range cases {
		code, _, errOut := call(c.env, c.args...)
		if code != 7 || len(got) != i+1 || got[i] != c.want || deps[i].Home != home || deps[i].Submitter != c.submitter {
			t.Errorf("%v: exit %d stderr %q options %+v deps %+v", c.args, code, errOut, got, deps)
		}
	}
}

package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/runenv"
)

func TestRunEnvWithoutAProjectOrWithAnUnknownFlagPrintsItsUsageAndExits2(t *testing.T) {
	for _, args := range [][]string{{"run-env"}, {"run-env", "--project"}, {"run-env", "--project", ".", "--unsealed"}} {
		code, out, errOut := call(nil, args...)
		if code != 2 || out != "" || !strings.Contains(errOut, "usage: cockpit run-env --project <path>") {
			t.Errorf("%v: exit %d stdout %q stderr %q", args, code, out, errOut)
		}
	}
	if code, _, errOut := call(nil, "fly"); code != 2 || !strings.Contains(errOut, "run-env --project") {
		t.Fatalf("the top-level usage does not list run-env: %q", errOut)
	}
}

func TestRunEnvFlagsReachTheRunOptionsAndTheStateDirectoryReachesItsDeps(t *testing.T) {
	previous := runEnvRun
	t.Cleanup(func() { runEnvRun = previous })
	var got []runenv.Options
	var homes []string
	runEnvRun = func(_ context.Context, opts runenv.Options, deps runenv.Deps) int {
		got, homes = append(got, opts), append(homes, deps.Home)
		return 7
	}
	home, project := t.TempDir(), t.TempDir()
	cases := []struct {
		args []string
		want runenv.Options
	}{
		{[]string{"run-env", "--project", project}, runenv.Options{Project: project}},
		{[]string{"run-env", "--project", project, "--suite", "smoke", "--unseal-check", "--keep"}, runenv.Options{Project: project, Suite: "smoke", UnsealCheck: true, Keep: true}},
		{[]string{"run-env", "--keep", "--project", filepath.Join(project, "."), "--unseal-check"}, runenv.Options{Project: project, UnsealCheck: true, Keep: true}},
	}
	if len(cases) != 3 {
		t.Fatalf("%d cases", len(cases))
	}
	for i, c := range cases {
		code, _, errOut := call(map[string]string{"COCKPIT_HOME": home}, c.args...)
		if code != 7 || len(got) != i+1 || got[i] != c.want || homes[i] != home {
			t.Errorf("%v: exit %d stderr %q options %+v home %v", c.args, code, errOut, got, homes)
		}
	}
}

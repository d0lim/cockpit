package main

import (
	"strings"
	"testing"
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

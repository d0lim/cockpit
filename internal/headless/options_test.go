package headless

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRunArgumentsAcceptProjectSuiteOutputsAndFlagsAndRejectEverythingElse(t *testing.T) {
	o, err := Parse([]string{"--project", "toy", "--suite", "smoke", "--unseal-check", "--keep", "--out", "bundle", "--archive", "run.tar.gz"})
	if err != nil {
		t.Fatal(err)
	}
	abs := func(p, suffix string) bool { return filepath.IsAbs(p) && strings.HasSuffix(p, suffix) }
	if !abs(o.Project, "/toy") || !abs(o.Out, "/bundle") || !abs(o.Archive, "/run.tar.gz") || o.Suite != "smoke" || !o.UnsealCheck || !o.Keep {
		t.Fatalf("options %+v", o)
	}
	bad := map[string][]string{
		"no project":       {"--keep"},
		"project no value": {"--project"},
		"out no value":     {"--project", "x", "--out"},
		"archive empty":    {"--project", "x", "--archive", ""},
		"same path":        {"--project", "x", "--out", "a", "--archive", "a"},
		"unknown flag":     {"--project", "x", "--force"},
		"bad suite":        {"--project", "x", "--suite", "../etc"},
		"uppercase suite":  {"--project", "x", "--suite", "Smoke"},
	}
	rejected := 0
	for name, args := range bad {
		if _, err := Parse(args); err == nil {
			t.Errorf("%s: %v passed", name, args)
		} else {
			rejected++
		}
	}
	if rejected != 8 {
		t.Fatalf("%d of 8 bad argument lists were rejected", rejected)
	}
}

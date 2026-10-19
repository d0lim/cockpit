package runenv

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRunEnvArgumentsAcceptProjectSuiteAndFlagsAndRejectEverythingElse(t *testing.T) {
	o, err := Parse([]string{"--project", "toy", "--suite", "smoke", "--unseal-check", "--keep"})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(o.Project) || !strings.HasSuffix(o.Project, "/toy") || o.Suite != "smoke" || !o.UnsealCheck || !o.Keep {
		t.Fatalf("options %+v", o)
	}
	bad := map[string][]string{
		"no project":       {"--keep"},
		"project no value": {"--project"},
		"unknown flag":     {"--project", "x", "--force"},
		"bad suite":        {"--project", "x", "--suite", "../etc"},
		"uppercase suite":  {"--project", "x", "--suite", "Smoke"},
	}
	for name, args := range bad {
		if _, err := Parse(args); err == nil {
			t.Errorf("%s: %v passed", name, args)
		}
	}
}

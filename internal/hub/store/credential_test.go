package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var credentialMutations = []porttest.Mutation{
	{"meta refuses credential file paths", "skip the credentialPathAt check in writeMeta", "red: meta.json is written with a .env path", "TestWriteMetaRefusesACredentialFilePathAndNamesTheField"},
	{"the refusal writes nothing", "write before checking", "red: meta.json exists after the refusal", "TestWriteMetaRefusesACredentialFilePathAndNamesTheField"},
	{"lines that mention a credential file are kept", "match without the whitespace guard", "red: a log line naming .env is refused", "TestWriteMetaKeepsLogLinesRunEnvAndBundlePaths"},
	{"credential file name pattern", "CredentialPathOnly returns false", "red: private env and secrets paths are not recognized", "TestCredentialPathOnlyMatchesWholeValueCredentialPathsOnly"},
}

func TestWriteMetaRefusesACredentialFilePathAndNamesTheField(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*Meta)
		field string
	}{
		{"notice with a .env path", func(m *Meta) { m.Notice = "/Users/someone/.local/state/cockpit/runs/" + runA + "/src/app/.env" }, "meta.notice"},
		{"moved-aside .env copy", func(m *Meta) { m.Notice = "src/app/.env.20261012.local" }, "meta.notice"},
		{"tail with a secrets file", func(m *Meta) { m.Tail = []string{"src/app/.secrets/secrets.yml"} }, "meta.tail[0]"},
		{"map value with a kit private env", func(m *Meta) { m.ImageKeys = map[string]string{"KIT": "env/e2e.private.env.json"} }, "meta.imageKeys.KIT"},
		{"notice with the runner copy of the private env", func(m *Meta) { m.Notice = "work/e2e/e2e.private.env.json" }, "meta.notice"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Open(homeFixture(t))
			err := s.WriteMeta(fixture(runA, c.edit))
			if err == nil || err.Error() != "meta does not record credential file paths: "+c.field {
				t.Fatalf("err %v", err)
			}
			dir, _ := s.RunDir(runA)
			if _, err := os.Stat(filepath.Join(dir, "meta.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("meta.json was written")
			}
		})
	}
	s := Open(homeFixture(t))
	if err := s.WriteMeta(fixture(runA)); err != nil {
		t.Fatalf("canary: clean meta refused: %v", err)
	}
}

func TestWriteMetaKeepsLogLinesRunEnvAndBundlePaths(t *testing.T) {
	s := Open(homeFixture(t))
	want := fixture(runA, func(m *Meta) {
		m.Tail = []string{"env file src/app/.env not found", "compose: exit 1"}
		m.Notice = "work/e2e/manifest.json"
		m.Redaction = &Redaction{Status: "failed", Files: []string{"junit/00-health.xml"}}
	})
	if err := s.WriteMeta(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadMeta(runA)
	if err != nil || !reflect.DeepEqual(*got, want) {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestCredentialPathOnlyMatchesWholeValueCredentialPathsOnly(t *testing.T) {
	deny := []string{
		"app/e2e.private.env.json",
		"work/e2e/e2e.private.env.json",
		"env/e2e.private.env.json",
		"/Users/x/.local/state/cockpit/runs/r/src/app/.env",
		"src/app/.secrets/secrets.yml",
		"src/app/certs/server.pem",
	}
	allow := []string{
		"work/e2e/manifest.json",
		"work/e2e/result.json",
		"work/e2e/ledger.json",
		"env/env.json",
		"  /x/app/.env",
		"COPY failed: /x/app/.env not found",
		"/x/build/libs/app.jar",
	}
	for _, v := range deny {
		if !CredentialPathOnly(v) {
			t.Errorf("credential path %q not recognized", v)
		}
	}
	for _, v := range allow {
		if CredentialPathOnly(v) {
			t.Errorf("%q recognized as a credential path", v)
		}
	}
	if len(deny)+len(allow) != 13 || !strings.HasSuffix(deny[0], ".private.env.json") {
		t.Fatal("case count")
	}
}

func TestCredentialMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "credential_test.go", credentialMutations)
}

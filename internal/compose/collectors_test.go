package compose

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var collectorMutations = []porttest.Mutation{
	{"collector ids reserved for the hub are refused", "drop the ReservedCollectorIDs check", "red: collectors named runner and logs pass", "TestCollectorsNamedRunnerOrLogsOrOutsideTheNamePatternAreRejectedAndOthersPass"},
	{"collector ids are path-safe names", "drop the namePattern check in checkCollectors", "red: a collector file named Bad_Name passes", "TestCollectorsNamedRunnerOrLogsOrOutsideTheNamePatternAreRejectedAndOthersPass"},
	{"secret source names become credential file patterns", "return no patterns from SecretFilePatterns", "red: a meta value naming app/.secrets.json is accepted", "TestSecretSourceAndEnvOutputNamesBecomeCredentialFilePatterns"},
}

func TestCollectorMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "collectors_test.go", collectorMutations)
}

func TestCollectorsNamedRunnerOrLogsOrOutsideTheNamePatternAreRejectedAndOthersPass(t *testing.T) {
	loaded := mustLoadToy(t)
	if got := strings.Join(CollectorIDs(loaded.Declaration, loaded.Tree), ","); got != "cache,facts,items" {
		t.Fatalf("toy collector ids %s", got)
	}
	evaluated := 0
	for _, name := range []string{"runner", "logs", "Bad_Name"} {
		_, err := loadToy(t, func(d string) {
			writeFile(t, filepath.Join(d, "scripts", "collectors", name+".ts"), "export {};\n")
		})
		r := rejection(t, err)
		if r.Kind != RejectConfinement || r.Field != "scripts/collectors/"+name+".ts" {
			t.Errorf("%s: %v", name, r)
		}
		evaluated++
	}
	if evaluated != 3 {
		t.Fatalf("evaluated %d", evaluated)
	}
	if _, err := loadToy(t, func(d string) {
		writeFile(t, filepath.Join(d, "scripts", "collectors", "runner-stats.ts"), "export {};\n")
	}); err != nil {
		t.Fatalf("a collector named runner-stats was rejected: %v", err)
	}
}

func TestSecretSourceAndEnvOutputNamesBecomeCredentialFilePatterns(t *testing.T) {
	decl := toyDeclaration(t)
	decl.Secrets.Sources = append(decl.Secrets.Sources, decl.Secrets.Sources[0])
	decl.Secrets.Sources[1].Path = "app/.secrets.json"
	patterns := SecretFilePatterns(decl)
	if len(patterns) != 3 {
		t.Fatalf("%d patterns", len(patterns))
	}
	for _, v := range []string{"app/.secrets.json", "/runs/r1/src/app/.secrets.json", ".secrets.json"} {
		if !store.CredentialPathOnly(v, patterns...) {
			t.Errorf("%s is not seen as a credential path", v)
		}
	}
	for _, v := range []string{"app/secrets.json.bak", "notes about .secrets.json", "app/x.secrets.json"} {
		if store.CredentialPathOnly(v, patterns...) {
			t.Errorf("%s is seen as a credential path", v)
		}
	}
	if store.CredentialPathOnly("app/.secrets.json") {
		t.Fatal("canary: the core list already knows .secrets.json, so the patterns prove nothing")
	}
}

package headless

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/cli"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var declarationMutations = []porttest.Mutation{
	{"cockpit run takes only a declaration file of the allowed shape", "assign the --declaration value without ParseDeclarationFile in Parse", "red: /etc/x.yaml, ../x.yaml and scripts/x.yaml parse", "TestRunArgumentsTakeADeclarationFileAndRefuseAnyOtherShape"},
	{"cockpit run reads the declaration it was given", "pass an empty DeclarationFile to ReadDeclaration in the declaration step", "red: the alt run reports the suite smoke", "TestARunOfAnotherDeclarationRunsItsSuiteAndItsBundleNamesItAndAMissingOneExitsThree"},
	{"the bundle of cockpit run names the declaration it read", "leave Declaration out of the run meta in Run", "red: meta.json of the alt run has no declaration", "TestARunOfAnotherDeclarationRunsItsSuiteAndItsBundleNamesItAndAMissingOneExitsThree"},
}

func TestDeclarationMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "declaration_test.go", declarationMutations)
}

func TestRunArgumentsTakeADeclarationFileAndRefuseAnyOtherShape(t *testing.T) {
	for value, want := range map[string]string{".cockpit/alt.yaml": ".cockpit/alt.yaml", ".cockpit/project.yaml": ""} {
		o, err := Parse([]string{"--project", "toy", "--declaration", value})
		if err != nil || string(o.Declaration) != want {
			t.Errorf("--declaration %s: %q, %v", value, o.Declaration, err)
		}
	}
	if o, err := Parse([]string{"--project", "toy"}); err != nil || o.Declaration != "" {
		t.Fatalf("no --declaration: %q, %v", o.Declaration, err)
	}
	refused := 0
	for _, value := range []string{"/etc/x.yaml", "../x.yaml", "scripts/x.yaml"} {
		if o, err := Parse([]string{"--project", "toy", "--declaration", value}); err == nil {
			t.Errorf("--declaration %s parsed as %q", value, o.Declaration)
		} else if strings.Contains(err.Error(), ".cockpit/<name>.yaml") {
			refused++
		}
	}
	if refused != 3 {
		t.Fatalf("refused %d of 3", refused)
	}
}

func TestARunOfAnotherDeclarationRunsItsSuiteAndItsBundleNamesItAndAMissingOneExitsThree(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	code, text := runFake(t, toyProject(t), newFake(nil), Options{Out: out, Declaration: ".cockpit/alt.yaml"})
	if code != cli.ExitOK || !strings.Contains(text, "suite alt, declaration .cockpit/alt.yaml") {
		t.Fatalf("exit %d\n%s", code, text)
	}
	summary := readJSON(t, filepath.Join(out, "summary.json"))
	meta := readJSON(t, filepath.Join(out, "meta.json"))
	if summary["suite"] != "alt" || meta["declaration"] != ".cockpit/alt.yaml" || summary["verdict"] != "passed" {
		t.Fatalf("summary suite %v verdict %v, meta declaration %v", summary["suite"], summary["verdict"], meta["declaration"])
	}
	plainOut := filepath.Join(t.TempDir(), "bundle")
	code, text = runFake(t, toyProject(t), newFake(nil), Options{Out: plainOut})
	if _, named := readJSON(t, filepath.Join(plainOut, "meta.json"))["declaration"]; code != cli.ExitOK || named || strings.Contains(text, "declaration .cockpit") {
		t.Fatalf("default run: exit %d, meta names a declaration %v\n%s", code, named, text)
	}
	code, text = runFake(t, toyProject(t), newFake(nil), Options{Declaration: ".cockpit/other.yaml"})
	if code != cli.ExitRejected || !strings.Contains(text, ".cockpit/other.yaml") {
		t.Fatalf("a declaration the commit lacks: exit %d\n%s", code, text)
	}
}

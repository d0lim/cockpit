package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var confineMutations = []porttest.Mutation{
	{"declared paths stay inside the commit", "drop the IsLocal and Clean checks in confinePath", "red: ../x and /abs pass in every path field", "TestEveryPathFieldWithParentSegmentsOrAnAbsolutePathIsRejectedWithItsLocation"},
	{"declared paths do not pass through committed links", "drop the tree.Link check in confinePath", "red: paths through a committed link pass", "TestPathsThroughACommittedSymbolicLinkAreRejectedWithTheLinkAndTheField"},
	{"names that become path segments match the name pattern", "drop the namePattern check in confine", "red: names with a slash, dots or uppercase pass", "TestNameFieldsWithASlashDotsOrUppercaseAreRejectedAndPatternNamesPass"},
	{"names are unique within their group", "drop the seen map in confine", "red: two runner sources named fixtures pass", "TestNameFieldsWithASlashDotsOrUppercaseAreRejectedAndPatternNamesPass"},
	{"suites and collectors stay inside scripts.dir", "drop the scripts.dir prefix check", "red: suites outside scripts.dir pass", "TestSuitesOrCollectorsOutsideTheScriptsDirectoryAreRejected"},
}

func TestConfineMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "confine_test.go", confineMutations)
}

func setPath(d *contract.Declaration, field string, value contract.RelPath) {
	switch field {
	case "compose.files[0]":
		d.Compose.Files[0] = value
	case "inputs.dirty[0]":
		d.Inputs.Dirty[0] = value
	case "inputs.runnerSources[0].path":
		d.Inputs.RunnerSources[0].Path = value
	case "images[0].inputs[0]":
		d.Images[0].Inputs[0] = value
	case "env.outputs[0]":
		d.Env.Outputs[0] = value
	case "secrets.sources[0].path":
		d.Secrets.Sources[0].Path = value
	case "scripts.dir":
		d.Scripts.Dir = value
	case "scripts.suites":
		d.Scripts.Suites = value
	case "scripts.collectors":
		d.Scripts.Collectors = value
	}
}

var pathFields = []string{
	"compose.files[0]", "inputs.dirty[0]", "inputs.runnerSources[0].path", "images[0].inputs[0]",
	"env.outputs[0]", "secrets.sources[0].path", "scripts.dir", "scripts.suites", "scripts.collectors",
}

func TestEveryPathFieldWithParentSegmentsOrAnAbsolutePathIsRejectedWithItsLocation(t *testing.T) {
	tree := mustLoadToy(t).Tree
	if err := confine(toyDeclaration(t), tree); err != nil {
		t.Fatalf("the toy declaration was confined: %v", err)
	}
	if got := len(declaredPaths(toyDeclaration(t))); got < len(pathFields) {
		t.Fatalf("declaredPaths lists %d paths, fewer than the %d fields under test", got, len(pathFields))
	}
	bad := []contract.RelPath{"../outside", "/etc/passwd", "a/../../b", "./mock", "mock/", `mock\..\x`}
	evaluated := 0
	for _, field := range pathFields {
		for _, value := range bad {
			d := toyDeclaration(t)
			setPath(&d, field, value)
			evaluated++
			err := confine(d, tree)
			if err == nil {
				t.Errorf("%s = %q passed", field, value)
				continue
			}
			if r := rejection(t, err); r.Kind != RejectConfinement || r.Field != field {
				t.Errorf("%s = %q: %v", field, value, r)
			}
		}
	}
	if evaluated != len(pathFields)*len(bad) {
		t.Fatalf("evaluated %d cases", evaluated)
	}
}

func TestPathsThroughACommittedSymbolicLinkAreRejectedWithTheLinkAndTheField(t *testing.T) {
	cases := []struct {
		field string
		old   string
		new   string
	}{
		{"compose.files[1]", "files: [compose.yaml, compose.sealed-app.yaml]", "files: [compose.yaml, linked/compose.sealed-app.yaml]"},
		{"inputs.runnerSources[0].path", "path: fixtures", "path: linked/fixtures"},
		{"images[0].inputs[0]", "inputs: [mock]", "inputs: [linked]"},
		{"env.outputs[1]", "outputs: [.env, e2e.private.env.json]", "outputs: [.env, escape/e2e.private.env.json]"},
		{"secrets.sources[0].path", "path: .env, format", "path: escape/.env, format"},
		{"scripts.dir", "dir: scripts", "dir: linked/scripts"},
	}
	evaluated := 0
	for _, c := range cases {
		_, err := loadToy(t, func(d string) {
			if err := os.Symlink(".", filepath.Join(d, "linked")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(os.TempDir(), filepath.Join(d, "escape")); err != nil {
				t.Fatal(err)
			}
			replaceIn(t, filepath.Join(d, DeclarationPath), c.old, c.new)
		})
		r := rejection(t, err)
		if r.Kind != RejectConfinement || r.Field != c.field || !strings.Contains(r.Message, "symbolic link") {
			t.Errorf("%s: %v", c.field, r)
		}
		evaluated++
	}
	if evaluated != len(cases) {
		t.Fatalf("evaluated %d cases", evaluated)
	}
	if _, err := loadToy(t, func(d string) {
		if err := os.Symlink(".", filepath.Join(d, "linked")); err != nil {
			t.Fatal(err)
		}
	}); err != nil {
		t.Fatalf("an unreferenced link in the commit made the declaration fail: %v", err)
	}
}

func TestNameFieldsWithASlashDotsOrUppercaseAreRejectedAndPatternNamesPass(t *testing.T) {
	tree := mustLoadToy(t).Tree
	set := map[string]func(d *contract.Declaration, v contract.Name){
		"ports[0].name":                func(d *contract.Declaration, v contract.Name) { d.Ports[0].Name = v },
		"inputs.runnerSources[0].name": func(d *contract.Declaration, v contract.Name) { d.Inputs.RunnerSources[0].Name = v },
		"databases[0].name":            func(d *contract.Declaration, v contract.Name) { d.Databases[0].Name = v },
	}
	bad := []contract.Name{"a/b", "..", ".", "App", "-a", "a_b", "", contract.Name("a" + strings.Repeat("b", 63))}
	good := []contract.Name{"app", "a-1", "0x", contract.Name("a" + strings.Repeat("b", 62))}
	rejected, accepted := 0, 0
	for field, apply := range set {
		for _, v := range bad {
			d := toyDeclaration(t)
			apply(&d, v)
			if r := rejection(t, confine(d, tree)); r.Kind != RejectConfinement || r.Field != field {
				t.Errorf("%s = %q: %v", field, v, r)
			}
			rejected++
		}
		for _, v := range good {
			d := toyDeclaration(t)
			apply(&d, v)
			if err := confine(d, tree); err != nil {
				t.Errorf("%s = %q was rejected: %v", field, v, err)
			}
			accepted++
		}
	}
	if rejected != len(set)*len(bad) || accepted != len(set)*len(good) {
		t.Fatalf("evaluated %d rejections and %d acceptances", rejected, accepted)
	}
	d := toyDeclaration(t)
	d.Inputs.RunnerSources = append(d.Inputs.RunnerSources, contract.RunnerSource{Name: "fixtures", Path: "mock"})
	if r := rejection(t, confine(d, tree)); r.Field != "inputs.runnerSources[1].name" || !strings.Contains(r.Message, "already used") {
		t.Fatalf("duplicate runner source name: %v", r)
	}
}

func TestSuitesOrCollectorsOutsideTheScriptsDirectoryAreRejected(t *testing.T) {
	tree := mustLoadToy(t).Tree
	d := toyDeclaration(t)
	d.Scripts.Suites = "mock"
	if r := rejection(t, confine(d, tree)); r.Field != "scripts.suites" {
		t.Fatalf("suites outside scripts.dir: %v", r)
	}
	d = toyDeclaration(t)
	d.Scripts.Collectors = "fixtures"
	if r := rejection(t, confine(d, tree)); r.Field != "scripts.collectors" {
		t.Fatalf("collectors outside scripts.dir: %v", r)
	}
	d = toyDeclaration(t)
	d.Scripts.Collectors = ""
	if err := confine(d, tree); err != nil {
		t.Fatalf("a declaration without collectors was rejected: %v", err)
	}
}

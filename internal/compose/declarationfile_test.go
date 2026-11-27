package compose

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const altDeclaration contract.DeclarationFile = ".cockpit/alt.yaml"

var declarationFileMutations = []porttest.Mutation{
	{"a run reads the declaration file it selected", "read DeclarationPath instead of file.Path() in ReadDeclaration", "red: the alt declaration loads scripts/suites", "TestARunReadsTheSelectedDeclarationAndItsWorkTreeHoldsThatFileAtTheFixedPath"},
	{"the work tree holds the selected declaration at the fixed path", "copy DeclarationPath instead of file.Path() in MakeWorkTree", "red: work/.cockpit/project.yaml of an alt run holds the default declaration", "TestARunReadsTheSelectedDeclarationAndItsWorkTreeHoldsThatFileAtTheFixedPath"},
	{"a refusal names the file the run selected", "name DeclarationPath in the missing and unreadable messages of ReadDeclaration", "red: a missing or broken .cockpit/alt.yaml is reported as .cockpit/project.yaml", "TestAMissingOrBrokenSelectedDeclarationIsRefusedNamingThatFileWhateverTheDefaultHolds"},
}

func TestDeclarationFileMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "declarationfile_test.go", declarationFileMutations)
}

func TestARunReadsTheSelectedDeclarationAndItsWorkTreeHoldsThatFileAtTheFixedPath(t *testing.T) {
	cases := []struct {
		file   contract.DeclarationFile
		suites contract.RelPath
		env    contract.RelPath
	}{
		{"", "scripts/suites", ""},
		{altDeclaration, "scripts/alt", "scripts/env.alt.json"},
	}
	for _, c := range cases {
		dir, sha := toyRepo(t, nil)
		loaded, err := ReadDeclaration(context.Background(), Git{Dir: dir}, sha, c.file)
		if err != nil {
			t.Fatalf("%q: %v", c.file, err)
		}
		if loaded.File != c.file || loaded.Declaration.Scripts.Suites != c.suites || loaded.Declaration.Scripts.Env != c.env {
			t.Fatalf("%q loaded file %q, suites %q, env %q", c.file, loaded.File, loaded.Declaration.Scripts.Suites, loaded.Declaration.Scripts.Env)
		}
		l := testLayout(t)
		root, err := OpenRun(l.Root)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		if err := Extract(context.Background(), loaded.Tree, root); err != nil {
			t.Fatal(err)
		}
		loaded.Declaration.Env.Outputs = loaded.Declaration.Env.Outputs[:1]
		if err := MakeWorkTree(root, loaded.Declaration, loaded.File); err != nil {
			t.Fatalf("%q: %v", c.file, err)
		}
		got := readFile(t, filepath.Join(l.Work, DeclarationPath))
		if want := readFile(t, filepath.Join(toyDir(t), c.file.Path())); got != want {
			t.Errorf("%q: work/%s holds\n%s\nwant\n%s", c.file, DeclarationPath, got, want)
		}
		entries, err := os.ReadDir(filepath.Join(l.Work, ".cockpit"))
		if err != nil || len(entries) != 1 {
			t.Errorf("%q: work/.cockpit holds %v, %v", c.file, entries, err)
		}
	}
}

func TestAMissingOrBrokenSelectedDeclarationIsRefusedNamingThatFileWhateverTheDefaultHolds(t *testing.T) {
	load := func(file contract.DeclarationFile, mutate func(dir string)) (Loaded, error) {
		dir, sha := toyRepo(t, mutate)
		return ReadDeclaration(context.Background(), Git{Dir: dir}, sha, file)
	}
	withoutDefault := func(d string) { os.Remove(filepath.Join(d, DeclarationPath)) }
	if loaded, err := load(altDeclaration, withoutDefault); err != nil || loaded.File != altDeclaration {
		t.Fatalf("the alt declaration of a commit without the default: %v", err)
	}
	if _, err := load("", withoutDefault); rejection(t, err).Kind != RejectMissing {
		t.Fatalf("the default declaration of a commit without it: %v", err)
	}
	cases := []struct {
		name   string
		file   contract.DeclarationFile
		mutate func(dir string)
		kind   RejectKind
	}{
		{"absent", ".cockpit/other.yaml", nil, RejectMissing},
		{"invalid YAML", altDeclaration, func(d string) { replaceIn(t, filepath.Join(d, altDeclaration.Path()), "project:\n", "project: [\n") }, RejectUnreadable},
		{"a link", altDeclaration, func(d string) {
			path := filepath.Join(d, altDeclaration.Path())
			os.Remove(path)
			if err := os.Symlink("project.yaml", path); err != nil {
				t.Fatal(err)
			}
		}, RejectUnreadable},
	}
	for _, c := range cases {
		_, err := load(c.file, c.mutate)
		r := rejection(t, err)
		if r.Kind != c.kind || !strings.Contains(r.Message, c.file.Path()) || strings.Contains(r.Message, DeclarationPath) {
			t.Errorf("%s: %v, want %s naming %s and not %s", c.name, r, c.kind, c.file.Path(), DeclarationPath)
		}
	}
}

package contract

import (
	"encoding/json"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var declarationFileMutations = []porttest.Mutation{
	{"only a file .cockpit/<name>.yaml is a declaration file", "return DeclarationFile(raw), nil before the pattern check in ParseDeclarationFile", "red: /.cockpit/x.yaml, .cockpit/../x.yaml and scripts/x.yaml pass", "TestADeclarationFileIsANamedYAMLFileUnderTheCockpitDirectoryAndEveryOtherShapeIsRefused"},
	{"the default path and an empty value are the same declaration", "return DeclarationFile(raw) for the default path in ParseDeclarationFile", "red: .cockpit/project.yaml parses to a value other than the zero value", "TestADeclarationFileIsANamedYAMLFileUnderTheCockpitDirectoryAndEveryOtherShapeIsRefused"},
	{"decoding a declaration file runs the same rule", "delete the UnmarshalText method of DeclarationFile", "red: a document that holds ../x.yaml decodes", "TestDecodingADeclarationFileRefusesWhatTheParseRefusesAndKeepsWhatItKeeps"},
}

func TestDeclarationFileMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "declarationfile_test.go", declarationFileMutations)
}

func TestADeclarationFileIsANamedYAMLFileUnderTheCockpitDirectoryAndEveryOtherShapeIsRefused(t *testing.T) {
	for raw, want := range map[string]string{"": DefaultDeclaration, DefaultDeclaration: DefaultDeclaration} {
		f, err := ParseDeclarationFile(raw)
		if err != nil || f != "" || f.Path() != want {
			t.Errorf("%q parsed to %q (path %q), %v; want the zero value that reads %s", raw, f, f.Path(), err, want)
		}
	}
	pairs := []struct{ refused, allowed string }{
		{"/.cockpit/x.yaml", ".cockpit/x.yaml"},
		{"/etc/x.yaml", ".cockpit/x.yaml"},
		{"../x.yaml", ".cockpit/x.yaml"},
		{".cockpit/../x.yaml", ".cockpit/x.yaml"},
		{"./.cockpit/x.yaml", ".cockpit/x.yaml"},
		{"scripts/x.yaml", ".cockpit/x.yaml"},
		{".cockpit/sub/x.yaml", ".cockpit/sub-x.yaml"},
		{".cockpit/X.yaml", ".cockpit/x.yaml"},
		{".cockpit/x.yml", ".cockpit/x.yaml"},
		{".cockpit/-x.yaml", ".cockpit/x-.yaml"},
		{".cockpit/x.yaml ", ".cockpit/x.yaml"},
		{`.cockpit\x.yaml`, ".cockpit/x.yaml"},
		{".cockpit/.yaml", ".cockpit/0.yaml"},
	}
	refused, allowed := 0, 0
	for _, p := range pairs {
		f, err := ParseDeclarationFile(p.allowed)
		if err != nil || f.Path() != p.allowed {
			t.Errorf("allowed twin %q: %q, %v", p.allowed, f, err)
		} else {
			allowed++
		}
		if f, err := ParseDeclarationFile(p.refused); err == nil {
			t.Errorf("%q passed as %q", p.refused, f)
		} else {
			refused++
		}
	}
	if refused != len(pairs) || allowed != len(pairs) || len(pairs) != 13 {
		t.Fatalf("%d refused and %d allowed of %d pairs", refused, allowed, len(pairs))
	}
}

func TestDecodingADeclarationFileRefusesWhatTheParseRefusesAndKeepsWhatItKeeps(t *testing.T) {
	type record struct {
		Declaration DeclarationFile `json:"declaration,omitempty"`
	}
	decode := func(doc string) (record, error) {
		var r record
		err := json.Unmarshal([]byte(doc), &r)
		return r, err
	}
	kept := map[string]DeclarationFile{`{}`: "", `{"declaration": ".cockpit/project.yaml"}`: "", `{"declaration": ".cockpit/alt.yaml"}`: ".cockpit/alt.yaml"}
	for doc, want := range kept {
		r, err := decode(doc)
		if err != nil || r.Declaration != want {
			t.Errorf("%s decoded to %q, %v; want %q", doc, r.Declaration, err, want)
		}
	}
	for _, doc := range []string{`{"declaration": "../x.yaml"}`, `{"declaration": "/etc/x.yaml"}`, `{"declaration": "scripts/x.yaml"}`} {
		if r, err := decode(doc); err == nil {
			t.Errorf("%s decoded to %q", doc, r.Declaration)
		}
	}
	data, err := json.Marshal(record{})
	if err != nil || string(data) != `{}` {
		t.Fatalf("a default declaration encodes as %s, %v", data, err)
	}
}

package archive

import (
	"errors"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var importDeclarationMutations = []porttest.Mutation{
	{"an imported bundle names only a declaration file of the allowed shape", "delete the UnmarshalText method of contract.DeclarationFile", "red: the metadata naming ../x.yaml unpacks", "TestAnImportedBundleKeepsItsDeclarationFileAndOneOfAnotherShapeIsRefused"},
}

func TestImportDeclarationMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "readdeclaration_test.go", importDeclarationMutations)
}

func TestAnImportedBundleKeepsItsDeclarationFileAndOneOfAnotherShapeIsRefused(t *testing.T) {
	base := exported(t)
	withDeclaration := func(value string) []entry {
		body := strings.Replace(entryBody(base, "meta.json"), "{", `{"declaration":"`+value+`",`, 1)
		return indexed(t, replaced(base, "meta.json", body), func(a *contract.Archive) {
			for i := range a.Files {
				if a.Files[i].Path == DirReport+"/meta.json" {
					a.Files[i].Bytes = int64(len(body))
				}
			}
		})
	}
	got, _, err := importInto(t, tarOf(t, withDeclaration(".cockpit/alt.yaml")))
	if err != nil || got.Meta.Declaration != ".cockpit/alt.yaml" {
		t.Fatalf("the bundle of an alt run imported as %q, %v", got.Meta.Declaration, err)
	}
	refused := 0
	for _, value := range []string{"../x.yaml", "/etc/x.yaml", "scripts/x.yaml"} {
		_, _, err := importInto(t, tarOf(t, withDeclaration(value)))
		var invalid *Invalid
		if !errors.As(err, &invalid) {
			t.Errorf("declaration %q: want a refusal, got %v", value, err)
			continue
		}
		refused++
	}
	if refused != 3 {
		t.Fatalf("refused %d of 3", refused)
	}
}

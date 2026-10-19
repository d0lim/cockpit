package seal

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"
)

var docRow = regexp.MustCompile("(?m)^\\| (\\d+) \\| `([a-z.-]+)` \\| ")

func TestSealingDocumentTableMatchesTheStaticAssertionsBothWays(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "sealing.md"))
	if err != nil {
		t.Fatal(err)
	}
	var documented []string
	for i, m := range docRow.FindAllStringSubmatch(string(data), -1) {
		if m[1] != strconv.Itoa(i+1) {
			t.Errorf("row %d is numbered %s", i+1, m[1])
		}
		documented = append(documented, m[2])
	}
	var code []string
	for _, r := range Static(toyInput(t)) {
		code = append(code, r.Name)
	}
	if len(documented) != 24 || !slices.Equal(documented, code) {
		t.Fatalf("docs/sealing.md lists %d assertions\n%q\ncode returns\n%q", len(documented), documented, code)
	}
	if !slices.Equal(code, assertionNames) {
		t.Fatalf("the test table drifted from the code: %q", code)
	}
}

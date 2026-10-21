package compose

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestSuitesListOnlyFilesWhoseNameIsASuiteName(t *testing.T) {
	loaded, err := loadToy(t, func(d string) {
		for _, name := range []string{"smoke.test.ts", "Upper.ts", "nightly-2.ts", "notes.md"} {
			writeFile(t, filepath.Join(d, "scripts", "suites", name), "export {};\n")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := Suites(loaded.Declaration, loaded.Tree); !slices.Equal(got, []string{"nightly-2", "smoke"}) {
		t.Fatalf("suites %v", got)
	}
}

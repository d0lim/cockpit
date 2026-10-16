package source

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var sourceMutations = []porttest.Mutation{
	{"undeclared source names are rejected", "Read falls back to an empty root when the roots lookup fails", "red: an unknown name is not PORT_DENIED", "TestDeclaredFilesAreReadWhileUndeclaredNamesEscapingPathsAndLinksAreDenied"},
	{"path escapes are rejected", "remove the IsLocal check", "red: a ../ path reads a file outside the root", "TestDeclaredFilesAreReadWhileUndeclaredNamesEscapingPathsAndLinksAreDenied"},
	{"symbolic links are rejected", "replace Lstat with Stat", "red: the content of the link target comes back", "TestDeclaredFilesAreReadWhileUndeclaredNamesEscapingPathsAndLinksAreDenied"},
	{"size limit", "remove the maxBytes check", "red: a file over the limit is read without truncation", "TestFileOverTheSizeLimitIsPortLimitAndFileWithinItIsReadAsIs"},
	{"listing returns regular files only", "do not filter the ReadDir result", "red: subdirectory and link names appear in the list", "TestListReturnsSortedRegularFileNamesDirectlyBelowAndEmptyForMissingDir"},
}

func fixture(t *testing.T) (*Reader, string) {
	t.Helper()
	root := t.TempDir()
	outside := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "modules"), 0o755))
	must(os.WriteFile(filepath.Join(root, "modules", "b.json"), []byte(`{"b":1}`), 0o644))
	must(os.WriteFile(filepath.Join(root, "modules", "a.json"), []byte(`{"a":1}`), 0o644))
	must(os.WriteFile(filepath.Join(root, "modules", ".hidden.json"), []byte(`{}`), 0o644))
	must(os.MkdirAll(filepath.Join(root, "modules", "sub"), 0o755))
	must(os.WriteFile(filepath.Join(outside, "secret.json"), []byte(`PLANTED_OUTSIDE`), 0o644))
	must(os.Symlink(filepath.Join(outside, "secret.json"), filepath.Join(root, "modules", "link.json")))
	must(os.WriteFile(filepath.Join(root, "big.json"), []byte(strings.Repeat("x", 65)), 0o644))
	r, err := Open(Roots{"fixtures": root}, 64, ports.Identity)
	must(err)
	t.Cleanup(r.Close)
	return r, outside
}

func codeOf(err error) engine.PortCode {
	var pe *engine.PortError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func TestDeclaredFilesAreReadWhileUndeclaredNamesEscapingPathsAndLinksAreDenied(t *testing.T) {
	r, _ := fixture(t)
	got, err := r.Read("fixtures", "modules/a.json")
	if err != nil || got != `{"a":1}` {
		t.Fatalf("allowed read: %q %v", got, err)
	}
	denied := []struct{ name, rel string }{
		{"other", "modules/a.json"},
		{"fixtures", "../secret.json"},
		{"fixtures", "/etc/hosts"},
		{"fixtures", "modules/link.json"},
	}
	for _, d := range denied {
		got, err := r.Read(d.name, d.rel)
		if codeOf(err) != engine.PortDenied || got != "" || strings.Contains(err.Error(), "PLANTED_OUTSIDE") {
			t.Fatalf("%s %s: %q %v", d.name, d.rel, got, err)
		}
	}
	if len(denied) != 4 {
		t.Fatal("number of denied cases")
	}
}

func TestFileOverTheSizeLimitIsPortLimitAndFileWithinItIsReadAsIs(t *testing.T) {
	r, _ := fixture(t)
	if _, err := r.Read("fixtures", "big.json"); codeOf(err) != engine.PortLimit {
		t.Fatalf("over the limit: %v", err)
	}
	got, err := r.Read("fixtures", "modules/b.json")
	if err != nil || got != `{"b":1}` {
		t.Fatalf("within the limit: %q %v", got, err)
	}
}

func TestListReturnsSortedRegularFileNamesDirectlyBelowAndEmptyForMissingDir(t *testing.T) {
	r, _ := fixture(t)
	names, err := r.List("fixtures", "modules")
	if err != nil || !slices.Equal(names, []string{".hidden.json", "a.json", "b.json"}) {
		t.Fatalf("list: %q %v", names, err)
	}
	empty, err := r.List("fixtures", "vectors")
	if err != nil || len(empty) != 0 {
		t.Fatalf("missing directory: %q %v", empty, err)
	}
	if _, err := r.List("fixtures", "../"); codeOf(err) != engine.PortDenied {
		t.Fatalf("escaping list: %v", err)
	}
	if _, err := r.List("other", "modules"); codeOf(err) != engine.PortDenied {
		t.Fatalf("list of an unknown name: %v", err)
	}
}

func TestSourceMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "source_test.go", sourceMutations)
}

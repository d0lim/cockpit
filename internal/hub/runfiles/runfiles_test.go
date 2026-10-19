package runfiles

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var runfilesMutations = []porttest.Mutation{
	{"runner files are read without following links", "drop the Lstat and fstat regular-file checks and O_NOFOLLOW", "red: a manifest link to a file outside the run is read", "TestRunnerFilesThatAreLinksOrTooLargeAreRefusedAndRegularFilesAreRead"},
	{"runner files are size-limited", "drop the Lstat and read size checks", "red: a 5 MiB result is read", "TestRunnerFilesThatAreLinksOrTooLargeAreRefusedAndRegularFilesAreRead"},
	{"exec result names cannot leave exec/", "skip ValidExecName in Exec", "red: ../result.json is read through Exec", "TestExecResultNamesOutsideThePatternAreRefusedAndAMissingLedgerIsEmpty"},
}

func TestRunfilesMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "runfiles_test.go", runfilesMutations)
}

const planted = "PLANTED-secret-7f3a9c"

func runRoot(t *testing.T) (string, *os.Root) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, E2EDir, ExecDir), 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return dir, root
}

func write(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func kind(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

func TestRunnerFilesThatAreLinksOrTooLargeAreRefusedAndRegularFilesAreRead(t *testing.T) {
	dir, root := runRoot(t)
	e2e := filepath.Join(dir, E2EDir)
	write(t, filepath.Join(e2e, "manifest.json"), contract.Manifest{SchemaVersion: contract.RunnerSchemaVersion, Suite: "smoke"})
	m, err := Manifest(root)
	if err != nil || m.Suite != "smoke" {
		t.Fatalf("regular manifest: %+v %v", m, err)
	}
	outside := filepath.Join(t.TempDir(), "manifest.json")
	write(t, outside, contract.Manifest{SchemaVersion: contract.RunnerSchemaVersion, Suite: planted})
	os.Remove(filepath.Join(e2e, "manifest.json"))
	if err := os.Symlink(outside, filepath.Join(e2e, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	m, err = Manifest(root)
	if kind(err) != Unsafe || m != nil || strings.Contains(err.Error(), planted) {
		t.Fatalf("link out of the run: %+v %v", m, err)
	}
	write(t, filepath.Join(dir, "real.json"), contract.Result{SchemaVersion: contract.RunnerSchemaVersion})
	if err := os.Symlink(filepath.Join("..", "..", "real.json"), filepath.Join(e2e, "result.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Result(root); kind(err) != Unsafe {
		t.Fatalf("link inside the run: %v", err)
	}
	os.Remove(filepath.Join(e2e, "result.json"))
	if err := os.WriteFile(filepath.Join(e2e, "result.json"), []byte(strings.Repeat(" ", MaxBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Result(root); kind(err) != TooLarge {
		t.Fatalf("oversized result: %v", err)
	}
	write(t, filepath.Join(e2e, "result.json"), map[string]any{"schemaVersion": 1, "steps": []any{}})
	if _, err := Result(root); kind(err) != Parse {
		t.Fatalf("schema version 1: %v", err)
	}
	if _, err := Read(root, "../escape", 10); kind(err) != Unsafe {
		t.Fatalf("relative escape: %v", err)
	}
}

func TestExecResultNamesOutsideThePatternAreRefusedAndAMissingLedgerIsEmpty(t *testing.T) {
	dir, root := runRoot(t)
	l, err := Ledger(root)
	if err != nil || l == nil || len(l.Flows) != 0 {
		t.Fatalf("missing ledger: %+v %v", l, err)
	}
	if _, err := Manifest(root); !IsMissing(err) {
		t.Fatalf("missing manifest: %v", err)
	}
	write(t, filepath.Join(dir, E2EDir, "result.json"), contract.Result{SchemaVersion: contract.RunnerSchemaVersion})
	for _, name := range []string{"../result.json", "a/b.json", ".hidden.json", "x.txt", ""} {
		if _, err := Exec(root, name); kind(err) != Unsafe {
			t.Errorf("%q: %v", name, err)
		}
	}
	name := "2026-10-19T01-02-03-000Z-health.json"
	write(t, filepath.Join(dir, E2EDir, ExecDir, name), contract.ExecResult{Result: contract.Result{SchemaVersion: contract.RunnerSchemaVersion}, Target: "health"})
	got, err := Exec(root, name)
	if err != nil || got.Target != "health" {
		t.Fatalf("exec result: %+v %v", got, err)
	}
}

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const importedID = "run-20261019-010011-abc1234-h1"

type importHub struct {
	status int
	got    []byte
	header http.Header
}

func (h *importHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.got, _ = io.ReadAll(r.Body)
	h.header = r.Header.Clone()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(h.status)
	if h.status == http.StatusCreated {
		json.NewEncoder(w).Encode(map[string]any{"id": importedID, "verdict": "passed", "files": 31, "run": map[string]any{"id": importedID}})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"code": "CONFLICT_STATE", "message": importedID + ": a run with this id is already in the hub"})
}

func importIn(t *testing.T, h *importHub, cwd string, argv ...string) (run, string) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	var out, errOut bytes.Buffer
	io := IO{Getenv: func(k string) string { return map[string]string{EnvURL: srv.URL}[k] }, Cwd: cwd, Stdout: &out, Stderr: &errOut}
	code := Main(context.Background(), append([]string{"runs", "import"}, argv...), io)
	return run{code: code, stdout: out.String(), stderr: errOut.String()}, srv.URL
}

func TestImportSendsTheArchiveWithTheGuardHeadersAndPrintsTheRunPage(t *testing.T) {
	dir := t.TempDir()
	body := []byte("\x1f\x8b archive bytes")
	if err := os.WriteFile(filepath.Join(dir, "cockpit-run.tar.gz"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	h := &importHub{status: http.StatusCreated}
	got, base := importIn(t, h, dir, "cockpit-run.tar.gz")
	if got.code != ExitOK || !bytes.Equal(h.got, body) || h.header.Get("Content-Type") != "application/gzip" || h.header.Get("X-Cockpit") != "1" || h.header.Get("Origin") != base {
		t.Fatalf("import: %+v, headers %v", got, h.header)
	}
	if !strings.Contains(got.stdout, base+"/runs/"+importedID) || !strings.Contains(got.stdout, "passed") || !strings.Contains(got.stdout, "31 report files") {
		t.Fatalf("output %q", got.stdout)
	}
}

func TestImportOfARunTheHubHoldsIsAConflictAndAMissingFileIsAUsageError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.tar.gz"), []byte("x"), 0o600)
	h := &importHub{status: http.StatusConflict}
	if got, _ := importIn(t, h, dir, "a.tar.gz"); got.code != ExitConflict || !strings.Contains(got.stderr, "already in the hub") {
		t.Fatalf("conflict: %+v", got)
	}
	h.got = nil
	if got, _ := importIn(t, h, dir, "missing.tar.gz"); got.code != ExitRejected || h.got != nil || !strings.Contains(got.stderr, "missing.tar.gz") {
		t.Fatalf("missing file: %+v, sent %q", got, h.got)
	}
	if got, _ := importIn(t, h, dir); got.code != ExitRejected {
		t.Fatalf("no file: %+v", got)
	}
}

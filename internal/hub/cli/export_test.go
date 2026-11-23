package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var exportMutations = []porttest.Mutation{
	{"the archive lands whole under the asked name with mode 600", "create the file with mode 644 in save", "red: the file is readable by others", "TestExportWritesTheArchiveToTheRunNameOrTheAskedFileAndReportsItsSize"},
	{"an existing file is never replaced", "drop the Lstat check in export", "red: the file holds the archive", "TestExportNeverReplacesAnExistingFileAndKeepsNothingOfABrokenDownload"},
	{"a download that breaks off keeps nothing", "treat a copy error as a whole download in save", "red: the short file stays", "TestExportNeverReplacesAnExistingFileAndKeepsNothingOfABrokenDownload"},
	{"a hub refusal writes no file", "save the body whatever the status in export", "red: the failure body is saved as an archive", "TestExportNeverReplacesAnExistingFileAndKeepsNothingOfABrokenDownload"},
}

func TestExportMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "export_test.go", exportMutations)
}

type archiveHub struct {
	status  int
	body    []byte
	claimed string
	paths   []string
}

func (h *archiveHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.paths = append(h.paths, r.Method+" "+r.URL.Path)
	if h.status != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(h.status)
		json.NewEncoder(w).Encode(map[string]any{"code": "READ_REFUSED", "message": "the report bundle failed the redaction rescan", "files": []string{"signals/stream.jsonl"}})
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	if h.claimed != "" {
		w.Header().Set("Content-Length", h.claimed)
	}
	w.WriteHeader(http.StatusOK)
	w.Write(h.body)
}

func exportIn(t *testing.T, h *archiveHub, cwd string, argv ...string) run {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	var out, errOut bytes.Buffer
	io := IO{Getenv: func(k string) string { return map[string]string{EnvURL: srv.URL}[k] }, Cwd: cwd, Stdout: &out, Stderr: &errOut}
	code := Main(context.Background(), append([]string{"runs", "export"}, argv...), io)
	return run{code: code, stdout: out.String(), stderr: errOut.String()}
}

func listing(t *testing.T, dir string) []string {
	t.Helper()
	found, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range found {
		names = append(names, e.Name())
	}
	return names
}

func TestExportWritesTheArchiveToTheRunNameOrTheAskedFileAndReportsItsSize(t *testing.T) {
	body := []byte("\x1f\x8b archive bytes")
	h := &archiveHub{status: http.StatusOK, body: body}
	dir := t.TempDir()
	got := exportIn(t, h, dir, id)
	target := filepath.Join(dir, id+".tar.gz")
	data, err := os.ReadFile(target)
	info, statErr := os.Stat(target)
	if got.code != ExitOK || err != nil || statErr != nil || !bytes.Equal(data, body) || info.Mode().Perm() != 0o600 || !strings.Contains(got.stdout, target) || !strings.Contains(got.stdout, "16 bytes") {
		t.Fatalf("default name: %+v %v", got, err)
	}
	if names := listing(t, dir); len(names) != 1 || h.paths[0] != "GET /api/runs/"+id+"/export" {
		t.Fatalf("directory %q, request %q", names, h.paths)
	}
	asked := filepath.Join(t.TempDir(), "out", "picked.tgz")
	if err := os.Mkdir(filepath.Dir(asked), 0o700); err != nil {
		t.Fatal(err)
	}
	got = exportIn(t, h, dir, id, "--out", asked, "--json")
	var line map[string]any
	if err := json.Unmarshal([]byte(got.stdout), &line); err != nil || got.code != ExitOK || line["code"] != CodeOK || line["file"] != asked || line["bytes"] != float64(len(body)) || line["id"] != id {
		t.Fatalf("asked name: %+v %v", got, err)
	}
	if data, err := os.ReadFile(asked); err != nil || !bytes.Equal(data, body) {
		t.Fatalf("asked file: %v", err)
	}
}

func TestExportNeverReplacesAnExistingFileAndKeepsNothingOfABrokenDownload(t *testing.T) {
	body := []byte("\x1f\x8b archive bytes")
	dir := t.TempDir()
	target := filepath.Join(dir, id+".tar.gz")
	if err := os.WriteFile(target, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := &archiveHub{status: http.StatusOK, body: body}
	got := exportIn(t, h, dir, id)
	if data, _ := os.ReadFile(target); got.code != ExitRejected || string(data) != "mine" || len(h.paths) != 0 || !strings.Contains(got.stderr, "is not replaced") {
		t.Fatalf("an existing file: %+v", got)
	}
	short := t.TempDir()
	got = exportIn(t, &archiveHub{status: http.StatusOK, body: body, claimed: "4096"}, short, id)
	if names := listing(t, short); got.code != ExitError || len(names) != 0 || !strings.Contains(got.stderr, "nothing was kept") {
		t.Fatalf("a short download: %+v, directory %q", got, names)
	}
	refused := t.TempDir()
	got = exportIn(t, &archiveHub{status: http.StatusConflict}, refused, id)
	if names := listing(t, refused); got.code != ExitConflict || len(names) != 0 || !strings.Contains(got.stderr, "READ_REFUSED") || !strings.Contains(got.stderr, "signals/stream.jsonl") {
		t.Fatalf("a refusal: %+v, directory %q", got, names)
	}
	whole := t.TempDir()
	if got := exportIn(t, &archiveHub{status: http.StatusOK, body: body}, whole, id); got.code != ExitOK || len(listing(t, whole)) != 1 {
		t.Fatalf("a whole download: %+v", got)
	}
}

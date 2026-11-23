package screen

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/archive"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/report"
)

const importedID = "run-20261019-010011-abc1234-h1"

func artifactZip(t *testing.T) []byte {
	t.Helper()
	dir := t.TempDir()
	meta, _ := json.Marshal(contract.BundleMeta{ID: importedID, SHA: "abc1234", Suite: "toy", Harness: 8, HubCommit: "0e47e78", Submitter: "ci",
		CreatedAt: "2026-10-19T01:00:11.000Z", Verdict: contract.VerdictPassed, ReportedAt: "2026-10-19T01:09:00.000Z", CockpitVersion: "0.7.0", HostAPI: 1,
		RunnerImage: contract.RunnerImage{Tag: "cockpit-runner:abc"}})
	summary := `{"format":3,"runId":"` + importedID + `","suite":"toy","verdict":"passed","reasons":[]}`
	for name, data := range map[string]string{report.FileSummary: summary, report.FileMeta: string(meta)} {
		target := filepath.Join(dir, report.DirReport, name)
		os.MkdirAll(filepath.Dir(target), 0o700)
		os.WriteFile(target, []byte(data), 0o600)
	}
	m, _ := mask.New(mask.Options{Mode: mask.Hub})
	var tgz bytes.Buffer
	if _, err := archive.Write(&tgz, archive.Input{RunDir: dir, RunID: importedID, Mask: m, Now: time.Now(), Version: "0.7.0"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	entry, _ := w.Create("cockpit-run.tar.gz")
	entry.Write(tgz.Bytes())
	w.Close()
	return out.Bytes()
}

func (f *fixture) upload(field string, body []byte, htmx bool) *http.Response {
	f.t.Helper()
	var form bytes.Buffer
	mw := multipart.NewWriter(&form)
	part, _ := mw.CreateFormFile(field, "cockpit-joint.zip")
	part.Write(body)
	mw.Close()
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/runs/import", &form)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	return res
}

func TestTheRunsScreenImportsAnArtifactZipAndOpensTheRun(t *testing.T) {
	f := newFixture(t)
	_, _, page := f.get("/")
	if !strings.Contains(page, `hx-post="/runs/import"`) || !strings.Contains(page, `name="archive"`) || !strings.Contains(page, `hx-encoding="multipart/form-data"`) {
		t.Fatalf("the runs page offers no import form: %s", page)
	}
	zipped := artifactZip(t)
	res := f.upload("archive", zipped, true)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("HX-Redirect") != "/runs/"+importedID {
		t.Fatalf("htmx import: %d %v", res.StatusCode, res.Header)
	}
	m, err := f.st.ReadMeta(importedID)
	if err != nil || m == nil || m.Env != lifecycle.EnvDestroyed || m.DestroyReason != lifecycle.DestroyImported {
		t.Fatalf("imported record %+v, %v", m, err)
	}
	if status, _, overview := f.get("/runs/" + importedID); status != http.StatusOK || !strings.Contains(overview, "imported from a bundle archive") {
		t.Fatalf("overview: %d", status)
	}
	again := f.upload("archive", zipped, false)
	raw, _ := io.ReadAll(again.Body)
	again.Body.Close()
	if again.StatusCode != http.StatusConflict || !strings.Contains(string(raw), "already in the hub") {
		t.Fatalf("second import: %d %s", again.StatusCode, raw)
	}
}

func TestAScreenImportWithoutAnArchiveSaysSoAndAPlainPostIsRedirected(t *testing.T) {
	f := newFixture(t)
	res := f.upload("other", []byte("x"), true)
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(string(raw), "pick a bundle archive") || res.Header.Get("HX-Redirect") != "" {
		t.Fatalf("no archive field: %d %s", res.StatusCode, raw)
	}
	bad := f.upload("archive", []byte("not an archive"), true)
	raw, _ = io.ReadAll(bad.Body)
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest || !strings.Contains(string(raw), "neither a tar.gz nor a zip") {
		t.Fatalf("not an archive: %d %s", bad.StatusCode, raw)
	}
	plain := f.upload("archive", artifactZip(t), false)
	plain.Body.Close()
	if plain.StatusCode != http.StatusSeeOther || plain.Header.Get("Location") != "/runs/"+importedID {
		t.Fatalf("plain post: %d %v", plain.StatusCode, plain.Header)
	}
}

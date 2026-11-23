package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/report"
)

var exportMutations = []porttest.Mutation{
	{"the archive is written with the run masker", "pass a masker with no known values to archive.Write", "red: the planted value is in the archive", "TestTheExportAnswersTheBundleAsAnArchiveReadThroughTheRunMasker"},
	{"a run before its report has no archive", "drop the pending case in export", "red: the active run answers its archive", "TestTheExportIsRefusedBeforeTheReportAfterADiscardedBundleAndWithoutOne"},
	{"a discarded bundle is never exported", "drop the redaction-failed case in export", "red: the run whose rescan failed answers its files", "TestTheExportIsRefusedBeforeTheReportAfterADiscardedBundleAndWithoutOne"},
	{"an entry that is not a regular file refuses the export", "drop the Refusal case in export", "red: the linked bundle answers 500", "TestTheExportIsRefusedBeforeTheReportAfterADiscardedBundleAndWithoutOne"},
	{"the archive is not served without a run masker", "build a pattern masker when Deps.Mask is nil", "red: the export answers 200", "TestTheExportIsNotServedWithoutARunMasker"},
}

func TestExportMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "export_test.go", exportMutations)
}

var doneKept = lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvStopped, Verdict: lifecycle.VerdictFailed}

func plantedBundle(t *testing.T, dir string) {
	t.Helper()
	writeBundle(t, dir)
	for name, text := range map[string]string{
		"e2e/result.json":       `{"complete":true,"note":"refused for ` + planted + `"}` + "\n",
		"source/flows/alpha.ts": "export const hint = \"" + planted + "\";\n",
		"signals/stream.jsonl":  `{"seq":1,"ts":"2026-10-19T01:00:00.000Z","tsSource":"host","kind":"event","collector":"toy-db","attrs":{"table":"orders"},"event":{"body":"row ` + planted + `"}}` + "\n",
	} {
		target := filepath.Join(dir, report.DirReport, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func entries(t *testing.T, raw []byte) map[string]string {
	t.Helper()
	zipped, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("the answer is no gzip stream: %v\n%s", err, shown(raw))
	}
	out := map[string]string{}
	for r := tar.NewReader(zipped); ; {
		head, err := r.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(r)
		out[head.Name] = string(body)
	}
}

func shown(raw []byte) string {
	return strings.ToValidUTF8(string(raw[:min(len(raw), 160)]), "?")
}

func (f *fixture) export(id string) (*http.Response, []byte) {
	f.t.Helper()
	res, err := f.srv.Client().Get(f.srv.URL + "/api/runs/" + id + "/export")
	if err != nil {
		f.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res, raw
}

func TestTheExportAnswersTheBundleAsAnArchiveReadThroughTheRunMasker(t *testing.T) {
	f := newFixture(t, func(d *Deps) {
		d.Mask = func(string) *mask.Masker { return hubMasker(t, planted) }
	})
	f.seed(runMeta(1, doneKept, doneUp))
	plantedBundle(t, f.runDir(rid(1)))
	res, raw := f.export(rid(1))
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != ExportMedia || res.Header.Get("Content-Length") != strconv.Itoa(len(raw)) ||
		res.Header.Get("Content-Disposition") != `attachment; filename="`+rid(1)+`.tar.gz"` {
		t.Fatalf("export: %d %v", res.StatusCode, res.Header)
	}
	files := entries(t, raw)
	masked := 0
	for _, rel := range []string{"e2e/result.json", "source/flows/alpha.ts", "signals/stream.jsonl"} {
		text := files[rid(1)+"/report/"+rel]
		if strings.Contains(text, planted) || !strings.Contains(text, "****") {
			t.Errorf("%s: want the planted value gone and **** present in %q", rel, text)
		}
		masked++
	}
	var index struct {
		RunID string `json:"runId"`
		Files []any  `json:"files"`
	}
	if err := json.Unmarshal([]byte(files[rid(1)+"/index.json"]), &index); err != nil {
		t.Fatal(err)
	}
	if masked != 3 || len(files) != 12 || index.RunID != rid(1) || len(index.Files) != 11 || !strings.Contains(files[rid(1)+"/report/summary.json"], "alpha › a2") {
		t.Fatalf("%d entries, %d listed", len(files), len(index.Files))
	}
}

func TestTheExportIsRefusedBeforeTheReportAfterADiscardedBundleAndWithoutOne(t *testing.T) {
	f := newFixture(t, func(d *Deps) {
		d.Mask = func(string) *mask.Masker { return hubMasker(t, planted) }
	})
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}, doneUp))
	f.seed(runMeta(2, doneKept, doneUp, func(m *store.Meta) {
		m.Redaction = &store.Redaction{Status: report.RedactionFailed, Files: []string{"signals/stream.jsonl"}}
	}))
	f.seed(runMeta(3, doneKept, doneUp))
	f.seed(runMeta(4, doneKept, doneUp))
	f.seed(runMeta(5, doneKept, doneUp))
	for _, n := range []int{1, 2, 4, 5} {
		plantedBundle(t, f.runDir(rid(n)))
	}
	outside := filepath.Join(t.TempDir(), "run.env")
	os.WriteFile(outside, []byte("TOKEN="+planted+"\n"), 0o600)
	if err := os.Symlink(outside, filepath.Join(f.runDir(rid(4)), report.DirReport, "e2e", "context.json")); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		run    int
		status int
		code   string
		file   string
	}{
		{1, http.StatusConflict, CodeNotAvailable, ""},
		{2, http.StatusConflict, CodeReadRefused, "signals/stream.jsonl"},
		{3, http.StatusNotFound, CodeNotFound, ""},
		{4, http.StatusConflict, CodeReadRefused, "e2e/context.json"},
	}
	for _, c := range cases {
		res, raw := f.export(rid(c.run))
		var p Problem
		if err := json.Unmarshal(raw, &p); err != nil || res.StatusCode != c.status || p.Code != c.code || strings.Contains(string(raw), planted) {
			t.Errorf("run %d: %d %s", c.run, res.StatusCode, shown(raw))
		}
		if c.file != "" && (len(p.Files) != 1 || p.Files[0] != c.file) {
			t.Errorf("run %d: files %q", c.run, p.Files)
		}
	}
	if res, raw := f.export(rid(5)); res.StatusCode != http.StatusOK || len(entries(t, raw)) != 12 || len(cases) != 4 {
		t.Fatalf("a run with a bundle: %d", res.StatusCode)
	}
}

func TestTheExportIsNotServedWithoutARunMasker(t *testing.T) {
	bare := newFixture(t, nil)
	bare.seed(runMeta(1, doneKept, doneUp))
	plantedBundle(t, bare.runDir(rid(1)))
	res, raw := bare.export(rid(1))
	var p Problem
	if err := json.Unmarshal(raw, &p); err != nil || res.StatusCode != http.StatusConflict || p.Code != CodeNotAvailable || strings.Contains(string(raw), planted) {
		t.Fatalf("without a masker: %d %s", res.StatusCode, shown(raw))
	}
	masked := newFixture(t, func(d *Deps) {
		d.Mask = func(string) *mask.Masker { return hubMasker(t, planted) }
	})
	masked.seed(runMeta(1, doneKept, doneUp))
	plantedBundle(t, masked.runDir(rid(1)))
	if res, raw := masked.export(rid(1)); res.StatusCode != http.StatusOK || len(entries(t, raw)) != 12 {
		t.Fatalf("with a masker: %d", res.StatusCode)
	}
}

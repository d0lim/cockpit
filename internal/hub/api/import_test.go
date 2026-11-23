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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/archive"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/report"
)

var importMutations = []porttest.Mutation{
	{"an import body must be a bundle archive", "drop the media type check in importRun", "red: the archive sent as JSON is imported", "TestAnImportIsRefusedWithoutTheGuardAsAnotherMediaTypeOrUnmaskedAndLeavesNothing"},
	{"a refused import leaves no stage behind", "drop the deferred RemoveAll of the stage in importRun", "red: runs holds an .import- directory", "TestAnImportIsRefusedWithoutTheGuardAsAnotherMediaTypeOrUnmaskedAndLeavesNothing"},
	{"a file the default rules change is refused with its name masked", "answer an Unmasked refusal as 400 in importProblem", "red: the planted archive answers 400", "TestAnImportIsRefusedWithoutTheGuardAsAnotherMediaTypeOrUnmaskedAndLeavesNothing"},
	{"a run id the hub holds is a conflict", "drop the Lstat check in store.Adopt", "red: the second import answers 500", "TestAnImportedArchiveOpensAsADestroyedRunWithItsReportAndASecondImportConflicts"},
}

func TestImportMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "import_test.go", importMutations)
}

const importedID = "run-20261019-010011-abc1234-h1"

func importArchive(t *testing.T, result string) []byte {
	t.Helper()
	dir := t.TempDir()
	meta, _ := json.Marshal(contract.BundleMeta{ID: importedID, SHA: "abc1234", Suite: "toy", Harness: 8, HubCommit: "0e47e78", Submitter: "ci",
		CreatedAt: "2026-10-19T01:00:11.000Z", Verdict: contract.VerdictPassed, ReportedAt: "2026-10-19T01:09:00.000Z", CockpitVersion: "0.6.0", HostAPI: 1,
		RunnerImage: contract.RunnerImage{Tag: "cockpit-runner:abc"}})
	summary, _ := json.Marshal(contract.Summary{Format: contract.SummaryFormat3, RunID: importedID, Suite: "toy", Verdict: contract.VerdictPassed, Reasons: []string{},
		Flows: []contract.FlowSummary{}, NotProven: contract.NotProven{Skipped: []string{}, Declared: []string{}, Unread: []string{}}, Seal: contract.SealPassed,
		Stages: map[string]contract.CollectStage{}, Ports: map[string]int{}, Images: contract.SummaryImages{Keys: map[string]string{}, Digests: map[string]string{}}, Versions: map[string]string{}})
	for name, data := range map[string]string{report.FileSummary: string(summary), report.FileMeta: string(meta), "e2e/result.json": result} {
		target := filepath.Join(dir, report.DirReport, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(target), 0o700)
		if err := os.WriteFile(target, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if _, err := archive.Write(&out, archive.Input{RunDir: dir, RunID: importedID, Mask: hubMasker(t), Now: time.Now(), Version: "0.6.0"}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func (f *fixture) importBody(body []byte, header http.Header) (int, []byte) {
	f.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/api/runs/import", bytes.NewReader(body))
	req.Header = header
	res, err := f.srv.Client().Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, raw
}

func (f *fixture) gzipHeader() http.Header {
	return http.Header{"Origin": {f.origin}, HeaderCockpit: {"1"}, "Content-Type": {ExportMedia}}
}

func TestAnImportedArchiveOpensAsADestroyedRunWithItsReportAndASecondImportConflicts(t *testing.T) {
	f := newFixture(t, nil)
	body := importArchive(t, `{"complete":true}`)
	code, raw := f.importBody(body, f.gzipHeader())
	if code != http.StatusCreated {
		t.Fatalf("import: %d %s", code, raw)
	}
	got := validate(t, reflect.TypeFor[Imported](), raw)
	if got["id"] != importedID || got["verdict"] != "passed" || got["files"] != float64(3) {
		t.Fatalf("imported %v", got)
	}
	code, raw = f.get("/api/runs/" + importedID)
	var detail RunDetail
	json.Unmarshal(raw, &detail)
	if code != http.StatusOK || detail.Phase != lifecycle.Done || detail.Env != lifecycle.EnvDestroyed || detail.DestroyReason != lifecycle.DestroyImported {
		t.Fatalf("run: %d %s", code, raw)
	}
	code, raw = f.get("/api/runs/" + importedID + "/report")
	if code != http.StatusOK || !strings.Contains(string(raw), `"status":"bundle"`) {
		t.Fatalf("report: %d %s", code, raw)
	}
	code, raw = f.importBody(body, f.gzipHeader())
	if code != http.StatusConflict || !strings.Contains(string(raw), string(ticker.CodeConflictState)) {
		t.Fatalf("second import: %d %s", code, raw)
	}
}

func TestAnImportIsRefusedWithoutTheGuardAsAnotherMediaTypeOrUnmaskedAndLeavesNothing(t *testing.T) {
	f := newFixture(t, nil)
	clean := importArchive(t, `{"complete":true}`)
	plantedBody := retar(t, clean, "report/e2e/result.json", `{"complete":true,"note":"password: `+planted+`"}`)
	noGuard := f.gzipHeader()
	noGuard.Del(HeaderCockpit)
	asJSON := f.gzipHeader()
	asJSON.Set("Content-Type", "application/json")
	cases := []struct {
		name   string
		body   []byte
		header http.Header
		code   int
		reason string
	}{
		{"no guard header", clean, noGuard, http.StatusForbidden, ""},
		{"another media type", clean, asJSON, http.StatusUnsupportedMediaType, CodeBadRequest},
		{"not gzip", []byte("not an archive"), f.gzipHeader(), http.StatusBadRequest, CodeBadRequest},
		{"unmasked", plantedBody, f.gzipHeader(), http.StatusConflict, CodeReadRefused},
	}
	for _, c := range cases {
		code, raw := f.importBody(c.body, c.header)
		if code != c.code || !strings.Contains(string(raw), c.reason) || strings.Contains(string(raw), planted) {
			t.Errorf("%s: %d %s", c.name, code, raw)
		}
	}
	_, raw := f.importBody(plantedBody, f.gzipHeader())
	if !strings.Contains(string(raw), "report/e2e/result.json") {
		t.Fatalf("the refusal does not name the file: %s", raw)
	}
	names, _ := os.ReadDir(filepath.Join(f.st.Home(), "runs"))
	if len(names) != 0 || len(cases) != 4 {
		t.Fatalf("runs holds %v after refused imports", names)
	}
	if code, raw := f.importBody(clean, f.gzipHeader()); code != http.StatusCreated {
		t.Fatalf("the clean archive is refused: %d %s", code, raw)
	}
}

func retar(t *testing.T, raw []byte, rel, body string) []byte {
	t.Helper()
	files := entries(t, raw)
	files[importedID+"/"+rel] = body
	var index contract.Archive
	if err := json.Unmarshal([]byte(files[importedID+"/index.json"]), &index); err != nil {
		t.Fatal(err)
	}
	for i := range index.Files {
		if index.Files[i].Path == rel {
			index.Files[i].Bytes = int64(len(body))
		}
	}
	encoded, _ := json.Marshal(index)
	files[importedID+"/index.json"] = string(encoded)
	var out bytes.Buffer
	zipped := gzip.NewWriter(&out)
	w := tar.NewWriter(zipped)
	for name, text := range files {
		w.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o600, Size: int64(len(text))})
		w.Write([]byte(text))
	}
	w.Close()
	zipped.Close()
	return out.Bytes()
}

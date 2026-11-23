package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/wire/wiretest"
)

type entry struct {
	name string
	body string
	typ  byte
	link string
}

func importMeta() contract.BundleMeta {
	return contract.BundleMeta{
		ID: runID, SHA: "abc1234def5678abc1234def5678abc1234def56", Suite: "toy", Harness: 8, HubCommit: "0e47e78", Submitter: "ci", CreatedAt: "2026-10-19T01:00:11.000Z",
		StartedAt: "2026-10-19T01:00:12.000Z", Verdict: contract.VerdictFailed, ReportedAt: "2026-10-19T01:09:00.000Z", CockpitVersion: "0.6.0", HostAPI: 1,
		RunnerImage: contract.RunnerImage{Tag: "cockpit-runner:abc", ID: "sha256:abc"}, FailedStage: "testing", Wire: true,
	}
}

func importBundle(t *testing.T) string {
	t.Helper()
	runDir := t.TempDir()
	meta, _ := json.Marshal(importMeta())
	for rel, text := range map[string]string{
		"summary.json":             `{"format":3,"runId":"` + runID + `","verdict":"failed","reasons":["alpha › a2 failed"]}` + "\n",
		"meta.json":                string(meta) + "\n",
		"not-proven.md":            "# Not proven\n",
		"seal.json":                `{"verdict":"passed"}` + "\n",
		"e2e/manifest.json":        `{"suite":"toy"}` + "\n",
		"e2e/result.json":          `{"complete":true}` + "\n",
		"source/source-index.json": `{"files":{"flows/alpha.ts":{"lines":1}}}` + "\n",
		"source/flows/alpha.ts":    "export const hint = 1;\n",
	} {
		put(t, runDir, rel, text)
	}
	wiretest.Seed(t, runDir, "seeded-call-value")
	return runDir
}

func exported(t *testing.T) []entry {
	t.Helper()
	var out bytes.Buffer
	if _, err := Write(&out, Input{RunDir: importBundle(t), RunID: runID, Mask: masker(t), Now: exportedAt, Version: "0.6.0"}); err != nil {
		t.Fatal(err)
	}
	zipped, err := gzip.NewReader(&out)
	if err != nil {
		t.Fatal(err)
	}
	var list []entry
	for r := tar.NewReader(zipped); ; {
		head, err := r.Next()
		if err == io.EOF {
			return list
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(r)
		list = append(list, entry{name: head.Name, body: string(body)})
	}
}

func tarOf(t *testing.T, list []entry) []byte {
	t.Helper()
	var out bytes.Buffer
	zipped := gzip.NewWriter(&out)
	w := tar.NewWriter(zipped)
	for _, e := range list {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		size := int64(len(e.body))
		if typ != tar.TypeReg {
			size = 0
		}
		if err := w.WriteHeader(&tar.Header{Name: e.name, Typeflag: typ, Linkname: e.link, Mode: 0o600, Size: size}); err != nil {
			t.Fatal(err)
		}
		if size > 0 {
			w.Write([]byte(e.body))
		}
	}
	w.Close()
	zipped.Close()
	return out.Bytes()
}

func indexed(t *testing.T, list []entry, edit func(*contract.Archive)) []entry {
	t.Helper()
	out := []entry{}
	for _, e := range list {
		if e.name == runID+"/"+FileIndex {
			var index contract.Archive
			if err := json.Unmarshal([]byte(e.body), &index); err != nil {
				t.Fatal(err)
			}
			edit(&index)
			raw, _ := json.Marshal(index)
			e.body = string(raw)
		}
		out = append(out, e)
	}
	return out
}

func listed(path, kind, body string) func(*contract.Archive) {
	return func(index *contract.Archive) {
		index.Files = append(index.Files, contract.ArchiveFile{Path: path, Kind: contract.ArchiveKind(kind), Bytes: int64(len(body))})
	}
}

func importInto(t *testing.T, raw []byte) (Imported, string, error) {
	t.Helper()
	parent := t.TempDir()
	stage := filepath.Join(parent, ".import-test")
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := Unpack(bytes.NewReader(raw), stage, masker(t))
	names, _ := os.ReadDir(parent)
	if len(names) != 1 {
		t.Fatalf("the import wrote beside its stage: %v", names)
	}
	return got, stage, err
}

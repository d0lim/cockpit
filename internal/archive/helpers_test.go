package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/wire/wiretest"
)

const (
	planted = "PLANTED-archive-secret-7f3a91"
	runID   = "run-20261019-010011-abc1234"
	marker  = "****"
)

var exportedAt = time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)

func masker(t *testing.T, known ...string) *mask.Masker {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: known})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func put(t *testing.T, runDir, rel, text string) {
	t.Helper()
	target := filepath.Join(runDir, DirReport, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func bundleFiles() map[string]string {
	note := `"note":"seen ` + planted + `"`
	return map[string]string{
		"summary.json":             `{"format":3,"verdict":"failed","reasons":["a step failed","alpha › a2 — refused for ` + planted + `"]}` + "\n",
		"meta.json":                `{"id":"` + runID + `",` + note + `}` + "\n",
		"not-proven.md":            "# Not proven\n\n- alpha › a3 after " + planted + "\n",
		"seal.json":                `{"verdict":"passed",` + note + `}` + "\n",
		"e2e/manifest.json":        `{"suite":"toy",` + note + `}` + "\n",
		"e2e/result.json":          `{"complete":true,` + note + `}` + "\n",
		"e2e/ledger.json":          `{"flows":{},` + note + `}` + "\n",
		"e2e/context.json":         `{"captures":{},` + note + `}` + "\n",
		"e2e/exec/alpha-1.json":    `{"flows":[],` + note + `}` + "\n",
		"source/source-index.json": `{"files":{"flows/alpha.ts":{"lines":1}},` + note + `}` + "\n",
		"source/flows/alpha.ts":    "export const hint = \"" + planted + "\";\n",
		"signals/stream.jsonl": `{"seq":1,"ts":"2026-10-19T01:00:00.000Z","tsSource":"host","kind":"event","collector":"toy-db","id":"orders:7","attrs":{"table":"orders"},"event":{"body":"row ` + planted + `"}}` + "\n" +
			`{"seq":2,"ts":"2026-10-19T01:00:01.000Z","tsSource":"host","kind":"event","collector":"logs","attrs":{"container":"orders"},"event":{"body":"log ` + planted + `"}}` + "\n",
	}
}

func bundle(t *testing.T) string {
	t.Helper()
	runDir := t.TempDir()
	for rel, text := range bundleFiles() {
		put(t, runDir, rel, text)
	}
	wiretest.Seed(t, runDir, planted)
	return runDir
}

type unpacked struct {
	files map[string]string
	index contract.Archive
	all   string
}

func (u unpacked) report(rel string) string { return u.files[runID+"/"+DirReport+"/"+rel] }

func unpack(t *testing.T, data []byte) unpacked {
	t.Helper()
	zipped, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	u := unpacked{files: map[string]string{}}
	var all strings.Builder
	for r := tar.NewReader(zipped); ; {
		head, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if head.Typeflag != tar.TypeReg || head.Mode != fileMode || !strings.HasPrefix(head.Name, runID+"/") {
			t.Fatalf("entry %q: type %d mode %o", head.Name, head.Typeflag, head.Mode)
		}
		u.files[head.Name] = string(body)
		all.WriteString(head.Name + "\n" + string(body))
	}
	u.all = all.String()
	if err := json.Unmarshal([]byte(u.files[runID+"/"+FileIndex]), &u.index); err != nil {
		t.Fatalf("index: %v", err)
	}
	return u
}

func export(t *testing.T, runDir string, m *mask.Masker) (unpacked, error) {
	t.Helper()
	var out bytes.Buffer
	index, err := Write(&out, Input{RunDir: runDir, RunID: runID, Mask: m, Now: exportedAt, Version: "0.0.0-test"})
	if err != nil {
		if out.Len() != 0 {
			t.Fatalf("a refused archive wrote %d bytes", out.Len())
		}
		return unpacked{}, err
	}
	u := unpack(t, out.Bytes())
	if len(u.index.Files) != len(index.Files) || u.index.RunID != runID || u.index.Format != contract.ArchiveFormat1 {
		t.Fatalf("the index in the archive differs from the one returned: %+v", u.index)
	}
	return u, nil
}

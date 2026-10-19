package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var sourceMutations = []porttest.Mutation{
	{"runner files are not followed through symlinks", "drop both the Lstat regular-file check and the SameFile comparison in readCapped", "red: a manifest symlink inside the directory is read as ok", "TestRunnerFilesThroughSymlinksOrOverTheSizeLimitAreUnsafeAndNotBundled"},
	{"runner files have a size limit", "drop the len(data) > MaxStateBytes check in readCapped", "red: a result over 4 MiB is read as ok", "TestRunnerFilesThroughSymlinksOrOverTheSizeLimitAreUnsafeAndNotBundled"},
	{"suite sources stay inside the suite directory", "read by filepath.Join(suiteDir, name) instead of through os.Root and drop IsLocal in SuiteSources", "red: ../secret.ts is copied into the bundle", "TestSuiteSourcesCopyOnlyLocalFilesTheManifestPointsTo"},
	{"decode errors are told apart as parse or schema", "return ReadSchema for every error in decodeFailure", "red: a truncated result reads as schema", "TestParseAndSchemaFailuresAreToldApart"},
	{"exec files must match the hub's name pattern", "drop the execName check in readExec", "red: an oddly named exec file is bundled", "TestRunnerFilesAreReadWithAStatusPerFileAndOnlyGoodOnesAreBundled"},
}

func TestSourceMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "sources_test.go", sourceMutations)
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	writeRaw(t, path, data)
}

func writeRaw(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerFilesAreReadWithAStatusPerFileAndOnlyGoodOnesAreBundled(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, RunnerManifest), manifestFixture())
	writeRaw(t, filepath.Join(dir, RunnerLedger), []byte(`{"schemaVersion":2,"flows":{}}`))
	writeJSON(t, filepath.Join(dir, RunnerExecDir, "20261019T010000Z-alpha.json"), map[string]any{"schemaVersion": 2})
	writeRaw(t, filepath.Join(dir, RunnerExecDir, "odd name.json"), []byte(`{}`))
	c := ReadRunnerFiles(dir)
	if c.Manifest.Value == nil || c.Result.Failure != ReadMissing {
		t.Fatalf("manifest %+v, result %+v", c.Manifest, c.Result)
	}
	keys := make([]string, 0, len(c.Files))
	for k := range c.Files {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	want := []string{"e2e/exec/20261019T010000Z-alpha.json", "e2e/ledger.json", "e2e/manifest.json"}
	if !slices.Equal(keys, want) {
		t.Fatalf("files %q", keys)
	}
	stages := map[string]contract.CollectStageStatus{}
	for k, v := range c.Stages {
		stages[k] = v.Status
	}
	wantStages := map[string]contract.CollectStageStatus{"runner.manifest.json": "ok", "runner.result.json": "absent", "runner.ledger.json": "ok", "runner.context.json": "absent", "runner.exec": "error"}
	if len(stages) != len(wantStages) {
		t.Fatalf("stages %v", stages)
	}
	for k, v := range wantStages {
		if stages[k] != v {
			t.Errorf("%s: %s, want %s", k, stages[k], v)
		}
	}
}

func TestParseAndSchemaFailuresAreToldApart(t *testing.T) {
	cases := map[string]struct {
		body string
		want ReadFailure
	}{
		"not json":         {`{"schemaVersion":`, ReadParse},
		"trailing data":    {`{"schemaVersion":2,"suite":"toy","complete":true,"startedAt":"x","steps":[]} {}`, ReadParse},
		"schema version 1": {`{"schemaVersion":1,"suite":"toy","complete":true,"startedAt":"x","steps":[]}`, ReadSchema},
		"unknown field":    {`{"schemaVersion":2,"suite":"toy","complete":true,"startedAt":"x","steps":[],"extra":1}`, ReadSchema},
		"unknown status":   {`{"schemaVersion":2,"suite":"toy","complete":true,"startedAt":"x","steps":[{"flow":"f","id":"s","status":"bogus","requests":0}]}`, ReadSchema},
	}
	for name, c := range cases {
		dir := t.TempDir()
		writeRaw(t, filepath.Join(dir, RunnerResult), []byte(c.body))
		if got := ReadRunnerFiles(dir).Result; got.Failure != c.want || got.Value != nil {
			t.Errorf("%s: %+v, want %s", name, got, c.want)
		}
	}
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, RunnerResult), resultFixture())
	if got := ReadRunnerFiles(dir).Result; got.Failure != ReadOK || len(got.Value.Steps) != 6 {
		t.Fatalf("a good result: %+v", got)
	}
	if len(cases) != 5 {
		t.Fatal("case count")
	}
}

func TestRunnerFilesThroughSymlinksOrOverTheSizeLimitAreUnsafeAndNotBundled(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, "real.json"), manifestFixture())
	if err := os.Symlink("real.json", filepath.Join(dir, RunnerManifest)); err != nil {
		t.Fatal(err)
	}
	writeRaw(t, filepath.Join(dir, RunnerResult), []byte(`{"pad":"`+strings.Repeat("x", MaxStateBytes+1)+`"}`))
	c := ReadRunnerFiles(dir)
	if c.Manifest.Failure != ReadUnsafe || c.Result.Failure != ReadUnsafe || len(c.Files) != 0 {
		t.Fatalf("manifest %s, result %s, files %d", c.Manifest.Failure, c.Result.Failure, len(c.Files))
	}
	for _, k := range []string{"runner.manifest.json", "runner.result.json"} {
		if c.Stages[k].Status != contract.CollectStageError {
			t.Errorf("%s: %+v", k, c.Stages[k])
		}
	}
	if j := Judge(Inputs{Manifest: c.Manifest, Result: c.Result, Boot: &contract.BootVerdict{OK: true}, Seal: contract.SealPassed, RunnerCode: intp(0)}); j.Rule != 6 || j.Reasons[0] != "the manifest could not be read safely" {
		t.Fatalf("judgement %+v", j)
	}
	allow := t.TempDir()
	writeJSON(t, filepath.Join(allow, RunnerManifest), manifestFixture())
	if got := ReadRunnerFiles(allow).Manifest; got.Failure != ReadOK {
		t.Fatalf("a regular manifest: %s", got.Failure)
	}
}

func TestSuiteSourcesCopyOnlyLocalFilesTheManifestPointsTo(t *testing.T) {
	parent := t.TempDir()
	suite := filepath.Join(parent, "suite")
	writeRaw(t, filepath.Join(suite, "flows", "alpha.ts"), []byte("line one\nline two\n"))
	writeRaw(t, filepath.Join(parent, "secret.ts"), []byte(planted))
	m := manifestFixture()
	m.Flows[1].Source = &contract.ManifestSource{File: "../secret.ts", Line: 1}
	m.Flows[2].Source = &contract.ManifestSource{File: "flows/missing.ts", Line: 1}
	files, stage := SuiteSources(suite, m)
	if string(files["source/flows/alpha.ts"]) != "line one\nline two\n" || len(files) != 2 || stage.Status != contract.CollectStageError {
		t.Fatalf("files %d, stage %+v", len(files), stage)
	}
	var index SourceIndex
	if err := json.Unmarshal(files[SourceIndexFile], &index); err != nil {
		t.Fatal(err)
	}
	if index.Files["flows/alpha.ts"].Lines != 3 || index.Omitted["../secret.ts"] == "" || index.Omitted["flows/missing.ts"] != "missing" {
		t.Fatalf("index %+v", index)
	}
	for _, data := range files {
		if strings.Contains(string(data), planted) {
			t.Fatal("a file outside the suite was copied")
		}
	}
}

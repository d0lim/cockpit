package contract

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

var runnerFiles = []string{"manifest", "result", "ledger", "context", "exec"}

func TestFourRealRunnerFilesAndOneSyntheticExecPassTheGeneratedSchemas(t *testing.T) {
	validated := 0
	for _, name := range runnerFiles {
		if err := loadSchema(t, name).Validate(v1(t, name)); err != nil {
			t.Fatalf("testdata/contract/v1/%s.json: %v", name, err)
		}
		validated++
	}
	if validated != 5 {
		t.Fatalf("validated %d files", validated)
	}
}

func TestSchemaRejectsRunnerFileCopiesWithSchemaVersion3(t *testing.T) {
	rejected := 0
	for _, name := range runnerFiles {
		schema := loadSchema(t, name)
		doc := v1(t, name)
		if err := schema.Validate(doc); err != nil {
			t.Fatalf("%s: original did not pass: %v", name, err)
		}
		doc["schemaVersion"] = 3
		err := schema.Validate(doc)
		if err == nil || !strings.Contains(err.Error(), "schemaVersion") {
			t.Fatalf("%s: schemaVersion 3 was not rejected: %v", name, err)
		}
		rejected++
	}
	if rejected != 5 {
		t.Fatalf("confirmed rejection for %d files", rejected)
	}
}

func TestSchemaAcceptsSchemaVersion2OptionalFieldsInRunnerFiles(t *testing.T) {
	manifest := v1(t, "manifest")
	manifest["schemaVersion"] = 2
	manifest["runner"] = map[string]any{"version": "0.1.0", "image": "cockpit-runner:0.1.0"}
	if err := loadSchema(t, "manifest").Validate(manifest); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"result", "exec"} {
		doc := v1(t, name)
		doc["schemaVersion"] = 2
		set(t, doc, "2026-10-15T00:07:25.904Z", "steps", 0, "startedAt")
		set(t, doc, "2026-10-15T00:07:25.912Z", "steps", 0, "finishedAt")
		if err := loadSchema(t, name).Validate(doc); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func format2(t *testing.T, name string) map[string]any {
	t.Helper()
	return readDoc(t, filepath.Join(repoRoot(t), "testdata", "contract", "format2", name+".json"))
}

func dropNulls(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if child == nil {
				delete(x, k)
				continue
			}
			dropNulls(child)
		}
	case []any:
		for _, child := range x {
			dropNulls(child)
		}
	}
}

func withPin(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	out := cloneDoc(t, doc)
	out["cockpitVersion"] = "0.1.0"
	out["hostApi"] = 1
	out["runnerImage"] = map[string]any{"tag": "cockpit-runner:0.1.0", "id": "sha256:" + strings.Repeat("ab", 32)}
	return out
}

func toFormat3(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	out := withPin(t, doc)
	delete(out, "opa")
	out["format"] = 3
	dropNulls(out)
	return out
}

func TestFormat3CopyOfRealFormat2SummaryPassesSchemaAndStrictDecode(t *testing.T) {
	schema := loadSchema(t, "summary")
	original := format2(t, "summary")
	doc := toFormat3(t, original)
	if err := schema.Validate(doc); err != nil {
		t.Fatal(err)
	}
	var got Summary
	if err := Decode(bytes.NewReader(encodeDoc(t, doc)), &got); err != nil {
		t.Fatal(err)
	}
	if want := len(original["flows"].([]any)); want == 0 || len(got.Flows) != want || got.Format != SummaryFormat3 {
		t.Fatalf("flows %d (original %d), format %d", len(got.Flows), want, got.Format)
	}
}

func TestSummaryFormat3RejectsOpaAndFormat2(t *testing.T) {
	schema := loadSchema(t, "summary")
	original := format2(t, "summary")
	if _, ok := original["opa"]; !ok {
		t.Fatal("real format 2 has no opa")
	}
	withOpa := toFormat3(t, original)
	withOpa["opa"] = original["opa"]
	if err := schema.Validate(withOpa); err == nil || !strings.Contains(err.Error(), "opa") {
		t.Fatalf("summary that still has opa passed the schema: %v", err)
	}
	var got Summary
	if err := Decode(bytes.NewReader(encodeDoc(t, withOpa)), &got); err == nil || !strings.Contains(err.Error(), `unknown field "opa"`) {
		t.Fatalf("summary that still has opa passed strict decode: %v", err)
	}
	older := toFormat3(t, original)
	older["format"] = 2
	if err := schema.Validate(older); err == nil || !strings.Contains(err.Error(), "format") {
		t.Fatalf("format 2 passed the schema: %v", err)
	}
}

func TestRealBundleMetaPassesMetaSchemaOnlyWithTheThreeNewFields(t *testing.T) {
	schema := loadSchema(t, "meta")
	original := format2(t, "meta")
	pinned := withPin(t, original)
	if err := schema.Validate(pinned); err != nil {
		t.Fatal(err)
	}
	var got BundleMeta
	if err := Decode(bytes.NewReader(encodeDoc(t, pinned)), &got); err != nil || got.SHA == "" || got.RunnerImage.ID == "" {
		t.Fatalf("could not read meta (%+v): %v", got, err)
	}
	for _, field := range []string{"cockpitVersion", "hostApi", "runnerImage"} {
		missing := cloneDoc(t, pinned)
		delete(missing, field)
		if err := schema.Validate(missing); err == nil || !strings.Contains(err.Error(), field) {
			t.Fatalf("meta without %s passed: %v", field, err)
		}
	}
}

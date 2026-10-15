package contract

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

var runnerFiles = []string{"manifest", "result", "ledger", "context", "exec"}

func Test러너_파일_실물_넷과_합성_exec_하나가_생성한_스키마를_통과한다(t *testing.T) {
	validated := 0
	for _, name := range runnerFiles {
		if err := loadSchema(t, name).Validate(v1(t, name)); err != nil {
			t.Fatalf("testdata/contract/v1/%s.json: %v", name, err)
		}
		validated++
	}
	if validated != 5 {
		t.Fatalf("검증한 파일 %d", validated)
	}
}

func Test러너_파일의_schemaVersion을_3으로_바꾼_사본은_스키마가_거부한다(t *testing.T) {
	rejected := 0
	for _, name := range runnerFiles {
		schema := loadSchema(t, name)
		doc := v1(t, name)
		if err := schema.Validate(doc); err != nil {
			t.Fatalf("%s: 원본이 통과하지 못했다: %v", name, err)
		}
		doc["schemaVersion"] = 3
		err := schema.Validate(doc)
		if err == nil || !strings.Contains(err.Error(), "schemaVersion") {
			t.Fatalf("%s: schemaVersion 3 이 거부되지 않았다: %v", name, err)
		}
		rejected++
	}
	if rejected != 5 {
		t.Fatalf("거부를 확인한 파일 %d", rejected)
	}
}

func Test러너_파일의_schemaVersion_2_선택_필드를_스키마가_받는다(t *testing.T) {
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

func Test실물_요약_format_2를_옮긴_format_3_사본은_스키마와_엄격_디코드를_지난다(t *testing.T) {
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
		t.Fatalf("플로우 %d(원본 %d), format %d", len(got.Flows), want, got.Format)
	}
}

func Test요약_format_3은_opa와_format_2를_거부한다(t *testing.T) {
	schema := loadSchema(t, "summary")
	original := format2(t, "summary")
	if _, ok := original["opa"]; !ok {
		t.Fatal("실물 format 2 에 opa 가 없다")
	}
	withOpa := toFormat3(t, original)
	withOpa["opa"] = original["opa"]
	if err := schema.Validate(withOpa); err == nil || !strings.Contains(err.Error(), "opa") {
		t.Fatalf("opa 가 남은 요약이 스키마를 지났다: %v", err)
	}
	var got Summary
	if err := Decode(bytes.NewReader(encodeDoc(t, withOpa)), &got); err == nil || !strings.Contains(err.Error(), `unknown field "opa"`) {
		t.Fatalf("opa 가 남은 요약이 엄격 디코드를 지났다: %v", err)
	}
	older := toFormat3(t, original)
	older["format"] = 2
	if err := schema.Validate(older); err == nil || !strings.Contains(err.Error(), "format") {
		t.Fatalf("format 2 가 스키마를 지났다: %v", err)
	}
}

func Test실물_번들_meta에_세_필드를_더한_사본만_meta_스키마를_지난다(t *testing.T) {
	schema := loadSchema(t, "meta")
	original := format2(t, "meta")
	pinned := withPin(t, original)
	if err := schema.Validate(pinned); err != nil {
		t.Fatal(err)
	}
	var got BundleMeta
	if err := Decode(bytes.NewReader(encodeDoc(t, pinned)), &got); err != nil || got.SHA == "" || got.RunnerImage.ID == "" {
		t.Fatalf("meta 를 읽지 못했다(%+v): %v", got, err)
	}
	for _, field := range []string{"cockpitVersion", "hostApi", "runnerImage"} {
		missing := cloneDoc(t, pinned)
		delete(missing, field)
		if err := schema.Validate(missing); err == nil || !strings.Contains(err.Error(), field) {
			t.Fatalf("%s 없는 meta 가 통과했다: %v", field, err)
		}
	}
}

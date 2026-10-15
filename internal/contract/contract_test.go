package contract

import (
	"bytes"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func v1(t *testing.T, name string) map[string]any {
	t.Helper()
	return readDoc(t, filepath.Join(repoRoot(t), "testdata", "contract", "v1", name+".json"))
}

func Test엄격_디코드는_결과_파일의_모르는_필드를_위치와_함께_거부한다(t *testing.T) {
	cases := []struct {
		name  string
		field string
		path  []any
	}{
		{"최상위", "extra", []any{"extra"}},
		{"단계 안", "retries", []any{"steps", 0, "retries"}},
	}
	for _, c := range cases {
		doc := v1(t, "result")
		set(t, doc, 1, c.path...)
		var got Result
		err := Decode(bytes.NewReader(encodeDoc(t, doc)), &got)
		if err == nil || !strings.Contains(err.Error(), `unknown field "`+c.field+`"`) {
			t.Fatalf("%s: 모르는 필드가 거부되지 않았다: %v", c.name, err)
		}
	}
}

func Test엄격_디코드는_선택_필드가_없는_v1_결과_파일을_받는다(t *testing.T) {
	doc := v1(t, "result")
	var got Result
	if err := Decode(bytes.NewReader(encodeDoc(t, doc)), &got); err != nil {
		t.Fatal(err)
	}
	want := len(doc["steps"].([]any))
	if want == 0 || len(got.Steps) != want {
		t.Fatalf("단계 수 %d, 원본 %d", len(got.Steps), want)
	}
	if got.Steps[0].StartedAt != "" || got.Steps[0].FinishedAt != "" {
		t.Fatalf("v1 단계에 없는 시각이 채워졌다: %+v", got.Steps[0])
	}
}

func Test엄격_디코드는_값_뒤에_남은_데이터를_거부한다(t *testing.T) {
	data := append(encodeDoc(t, v1(t, "ledger")), []byte(" {}")...)
	var got Ledger
	if err := Decode(bytes.NewReader(data), &got); !errors.Is(err, ErrTrailingData) {
		t.Fatalf("뒤에 붙은 값이 거부되지 않았다: %v", err)
	}
	if err := Decode(bytes.NewReader(append(encodeDoc(t, v1(t, "ledger")), '\n')), &got); err != nil {
		t.Fatalf("줄바꿈만 붙은 파일이 거부됐다: %v", err)
	}
}

type runnerRead func(data []byte) (int, error)

func readAs[T any, P interface {
	*T
	runnerFile
}](count func(*T) int) runnerRead {
	return func(data []byte) (int, error) {
		v, err := ReadRunnerFile[T, P](bytes.NewReader(data))
		if err != nil {
			return 0, err
		}
		return count(v), nil
	}
}

func Test허브_읽기는_러너_파일의_schemaVersion_2만_받고_나머지는_오류다(t *testing.T) {
	reads := map[string]runnerRead{
		"manifest": readAs(func(m *Manifest) int { return len(m.Flows) }),
		"result":   readAs(func(r *Result) int { return len(r.Steps) }),
		"ledger":   readAs(func(l *Ledger) int { return len(l.Flows) }),
		"context":  readAs(func(c *Context) int { return len(c.Captures) }),
		"exec":     readAs(func(e *ExecResult) int { return len(e.Steps) }),
	}
	checked := 0
	for name, read := range reads {
		doc := v1(t, name)
		for _, version := range []int{1, 3} {
			doc["schemaVersion"] = version
			if _, err := read(encodeDoc(t, doc)); !errors.Is(err, ErrSchemaVersion) {
				t.Fatalf("%s: schemaVersion %d 가 거부되지 않았다: %v", name, version, err)
			}
		}
		doc["schemaVersion"] = 2
		n, err := read(encodeDoc(t, doc))
		if err != nil || n == 0 {
			t.Fatalf("%s: schemaVersion 2 를 읽지 못했다(항목 %d): %v", name, n, err)
		}
		checked++
	}
	if checked != 5 {
		t.Fatalf("확인한 러너 파일 %d 종", checked)
	}
}

func schemaProperties(t *testing.T, name string) []string {
	t.Helper()
	props := readDoc(t, filepath.Join(repoRoot(t), "schema", name+".json"))["properties"].(map[string]any)
	var keys []string
	for k := range props {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func Test요약_format_3의_키는_format_2에서_opa를_빼고_세_필드를_더한_것이다(t *testing.T) {
	format2 := []string{"format", "runId", "suite", "verdict", "reasons", "steps", "flows", "stoppedAt", "notProven", "boot", "seal", "runner", "stages", "commit", "harness", "hubCommit", "ports", "images", "versions", "opa", "generatedAt"}
	want := sorted(append(slices.DeleteFunc(slices.Clone(format2), func(k string) bool { return k == "opa" }), "cockpitVersion", "hostApi", "runnerImage")...)
	if got := jsonNames(reflect.TypeFor[Summary]()); !slices.Equal(got, want) {
		t.Fatalf("Summary 필드 %v\n기대 %v", got, want)
	}
	if got := schemaProperties(t, "summary"); !slices.Equal(got, want) {
		t.Fatalf("schema/summary.json 속성 %v\n기대 %v", got, want)
	}
	if len(want) != len(format2)+2 {
		t.Fatalf("키 수 %d", len(want))
	}
}

func Test번들_meta의_키는_META_FIELDS에_id_verdict_reportedAt과_세_필드를_더한_것이다(t *testing.T) {
	metaFields := []string{"sha", "suite", "harness", "hubCommit", "submitter", "keep", "createdAt", "startedAt", "finishedAt", "slot", "imageKeys", "imageDigests", "failedStage", "notice", "tail"}
	want := sorted(append(slices.Clone(metaFields), "id", "verdict", "reportedAt", "cockpitVersion", "hostApi", "runnerImage")...)
	if got := jsonNames(reflect.TypeFor[BundleMeta]()); !slices.Equal(got, want) {
		t.Fatalf("BundleMeta 필드 %v\n기대 %v", got, want)
	}
	if got := schemaProperties(t, "meta"); !slices.Equal(got, want) {
		t.Fatalf("schema/meta.json 속성 %v\n기대 %v", got, want)
	}
	if got := jsonNames(reflect.TypeFor[RunnerImage]()); !slices.Equal(got, []string{"id", "tag"}) {
		t.Fatalf("runnerImage 필드 %v", got)
	}
}

func pointerFields(typ reflect.Type, seen map[reflect.Type]bool, visit func(owner reflect.Type, f reflect.StructField)) {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return
	}
	seen[typ] = true
	for _, f := range reflect.VisibleFields(typ) {
		if f.Type.Kind() == reflect.Pointer {
			visit(typ, f)
		}
		pointerFields(f.Type, seen, visit)
	}
}

func Test계약_구조체의_포인터_필드는_전부_omitempty라_스키마와_선언이_같은_선택성을_갖는다(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var checked []string
	for _, r := range Roots() {
		pointerFields(r.Type, seen, func(owner reflect.Type, f reflect.StructField) {
			name := owner.Name() + "." + f.Name
			checked = append(checked, name)
			if !slices.Contains(strings.Split(f.Tag.Get("json"), ",")[1:], "omitempty") {
				t.Errorf("%s 는 포인터인데 omitempty 가 없다", name)
			}
		})
	}
	for _, canary := range []string{"Manifest.Runner", "ResultStep.DurationMs", "Summary.Boot", "Signal.Span"} {
		if !slices.Contains(checked, canary) {
			t.Fatalf("%s 를 검사하지 않았다: %v", canary, checked)
		}
	}
}

func enumValues(t *testing.T, typ reflect.Type) []any {
	t.Helper()
	for _, e := range Enums() {
		if e.Type == typ {
			return e.Values
		}
	}
	t.Fatalf("%v 의 값 표가 없다", typ)
	return nil
}

func Test신호의_예약_속성과_닫힌_값_집합은_R26의_표와_같다(t *testing.T) {
	tables := []struct {
		typ  reflect.Type
		want []any
	}{
		{reflect.TypeFor[SignalKind](), []any{"metric", "event", "span"}},
		{reflect.TypeFor[TimeSource](), []any{"host", "container", "runner"}},
		{reflect.TypeFor[Tone](), []any{"ok", "warn", "bad", "idle", "info"}},
		{reflect.TypeFor[SpanStatus](), []any{"ok", "error", "incomplete"}},
	}
	for _, table := range tables {
		if got := enumValues(t, table.typ); !reflect.DeepEqual(got, table.want) {
			t.Fatalf("%v 값 %v, R26 %v", table.typ, got, table.want)
		}
	}
	if got := jsonNames(reflect.TypeFor[ReservedAttrs]()); !slices.Equal(got, sorted("table", "status", "label", "tone")) {
		t.Fatalf("예약 속성 %v", got)
	}
}

func signalDoc(kind string, body map[string]any) map[string]any {
	return map[string]any{
		"seq": 7, "ts": "2026-10-15T00:07:26.138Z", "tsSource": "host", "kind": kind, "collector": "orders",
		"attrs": map[string]any{"table": "orders", "status": "delivered", "label": "주문", "tone": "ok", "rows": 3, "late": false},
		kind:    body,
	}
}

func Test신호_스키마는_예약_속성_값과_스팬_상태와_속성_값의_꼴을_막는다(t *testing.T) {
	schema := loadSchema(t, "signal")
	span := map[string]any{"traceId": "t-1", "name": "주문", "start": "2026-10-15T00:07:26.138Z", "status": "incomplete", "links": []any{map[string]any{"traceId": "t-0", "spanId": "s-0"}}}
	valid := []map[string]any{
		signalDoc("event", map[string]any{"body": "{}"}),
		signalDoc("metric", map[string]any{"name": "orders.rows", "value": 3.5}),
		signalDoc("span", span),
	}
	valid[2]["id"], valid[2]["rev"] = "s-1", 2
	for i, doc := range valid {
		if err := schema.Validate(doc); err != nil {
			t.Fatalf("유효한 신호 %d 가 거부됐다: %v", i, err)
		}
	}
	mutations := []struct {
		name  string
		value any
		path  []any
	}{
		{"tone 밖 값", "purple", []any{"attrs", "tone"}},
		{"객체 속성 값", map[string]any{}, []any{"attrs", "nested"}},
		{"스팬 상태 밖 값", "done", []any{"span", "status"}},
		{"신호 종류 밖 값", "log", []any{"kind"}},
		{"시각 출처 밖 값", "browser", []any{"tsSource"}},
		{"모르는 머리 필드", 1, []any{"extra"}},
	}
	for _, m := range mutations {
		doc := signalDoc("span", span)
		doc["id"] = "s-1"
		set(t, doc, m.value, m.path...)
		if err := schema.Validate(doc); err == nil {
			t.Fatalf("%s 가 통과했다", m.name)
		}
	}
}

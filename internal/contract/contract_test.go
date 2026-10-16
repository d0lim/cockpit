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

func TestStrictDecodeRejectsUnknownResultFieldsAtTopLevelAndInsideSteps(t *testing.T) {
	cases := []struct {
		name  string
		field string
		path  []any
	}{
		{"top level", "extra", []any{"extra"}},
		{"inside a step", "retries", []any{"steps", 0, "retries"}},
	}
	for _, c := range cases {
		doc := v1(t, "result")
		set(t, doc, 1, c.path...)
		var got Result
		err := Decode(bytes.NewReader(encodeDoc(t, doc)), &got)
		if err == nil || !strings.Contains(err.Error(), `unknown field "`+c.field+`"`) {
			t.Fatalf("%s: unknown field was not rejected: %v", c.name, err)
		}
	}
}

func TestStrictDecodeAcceptsV1ResultFileWithoutOptionalFields(t *testing.T) {
	doc := v1(t, "result")
	var got Result
	if err := Decode(bytes.NewReader(encodeDoc(t, doc)), &got); err != nil {
		t.Fatal(err)
	}
	want := len(doc["steps"].([]any))
	if want == 0 || len(got.Steps) != want {
		t.Fatalf("steps %d, original %d", len(got.Steps), want)
	}
	if got.Steps[0].StartedAt != "" || got.Steps[0].FinishedAt != "" {
		t.Fatalf("v1 step got timestamps it does not carry: %+v", got.Steps[0])
	}
}

func TestStrictDecodeRejectsDataLeftAfterTheValue(t *testing.T) {
	data := append(encodeDoc(t, v1(t, "ledger")), []byte(" {}")...)
	var got Ledger
	if err := Decode(bytes.NewReader(data), &got); !errors.Is(err, ErrTrailingData) {
		t.Fatalf("trailing value was not rejected: %v", err)
	}
	if err := Decode(bytes.NewReader(append(encodeDoc(t, v1(t, "ledger")), '\n')), &got); err != nil {
		t.Fatalf("file with only a trailing newline was rejected: %v", err)
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

func TestHubReadAcceptsOnlySchemaVersion2RunnerFilesAndRejectsOthers(t *testing.T) {
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
				t.Fatalf("%s: schemaVersion %d was not rejected: %v", name, version, err)
			}
		}
		doc["schemaVersion"] = 2
		n, err := read(encodeDoc(t, doc))
		if err != nil || n == 0 {
			t.Fatalf("%s: could not read schemaVersion 2 (items %d): %v", name, n, err)
		}
		checked++
	}
	if checked != 5 {
		t.Fatalf("checked %d runner file kinds", checked)
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

func TestSummaryFormat3KeysAreFormat2KeysWithoutOpaPlusThreeFields(t *testing.T) {
	format2 := []string{"format", "runId", "suite", "verdict", "reasons", "steps", "flows", "stoppedAt", "notProven", "boot", "seal", "runner", "stages", "commit", "harness", "hubCommit", "ports", "images", "versions", "opa", "generatedAt"}
	want := sorted(append(slices.DeleteFunc(slices.Clone(format2), func(k string) bool { return k == "opa" }), "cockpitVersion", "hostApi", "runnerImage")...)
	if got := jsonNames(reflect.TypeFor[Summary]()); !slices.Equal(got, want) {
		t.Fatalf("Summary fields %v\nwant %v", got, want)
	}
	if got := schemaProperties(t, "summary"); !slices.Equal(got, want) {
		t.Fatalf("schema/summary.json properties %v\nwant %v", got, want)
	}
	if len(want) != len(format2)+2 {
		t.Fatalf("key count %d", len(want))
	}
}

func TestBundleMetaKeysAreMetaFieldsPlusIdVerdictReportedAtAndThreeFields(t *testing.T) {
	metaFields := []string{"sha", "suite", "harness", "hubCommit", "submitter", "keep", "createdAt", "startedAt", "finishedAt", "slot", "imageKeys", "imageDigests", "failedStage", "notice", "tail"}
	want := sorted(append(slices.Clone(metaFields), "id", "verdict", "reportedAt", "cockpitVersion", "hostApi", "runnerImage")...)
	if got := jsonNames(reflect.TypeFor[BundleMeta]()); !slices.Equal(got, want) {
		t.Fatalf("BundleMeta fields %v\nwant %v", got, want)
	}
	if got := schemaProperties(t, "meta"); !slices.Equal(got, want) {
		t.Fatalf("schema/meta.json properties %v\nwant %v", got, want)
	}
	if got := jsonNames(reflect.TypeFor[RunnerImage]()); !slices.Equal(got, []string{"id", "tag"}) {
		t.Fatalf("runnerImage fields %v", got)
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

func TestPointerFieldsInContractStructsAreAllOmitemptySoSchemaAndTypesAgreeOnOptionality(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var checked []string
	for _, r := range Roots() {
		pointerFields(r.Type, seen, func(owner reflect.Type, f reflect.StructField) {
			name := owner.Name() + "." + f.Name
			checked = append(checked, name)
			if !slices.Contains(strings.Split(f.Tag.Get("json"), ",")[1:], "omitempty") {
				t.Errorf("%s is a pointer without omitempty", name)
			}
		})
	}
	for _, canary := range []string{"Manifest.Runner", "ResultStep.DurationMs", "Summary.Boot", "Signal.Span"} {
		if !slices.Contains(checked, canary) {
			t.Fatalf("%s was not checked: %v", canary, checked)
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
	t.Fatalf("no value table for %v", typ)
	return nil
}

func TestSignalReservedAttrsAndClosedValueSetsMatchTheR26Table(t *testing.T) {
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
			t.Fatalf("%v values %v, R26 %v", table.typ, got, table.want)
		}
	}
	if got := jsonNames(reflect.TypeFor[ReservedAttrs]()); !slices.Equal(got, sorted("table", "status", "label", "tone")) {
		t.Fatalf("reserved attrs %v", got)
	}
}

func signalDoc(kind string, body map[string]any) map[string]any {
	return map[string]any{
		"seq": 7, "ts": "2026-10-15T00:07:26.138Z", "tsSource": "host", "kind": kind, "collector": "orders",
		"attrs": map[string]any{"table": "orders", "status": "delivered", "label": "order", "tone": "ok", "rows": 3, "late": false},
		kind:    body,
	}
}

func TestSignalSchemaRejectsBadReservedAttrValuesSpanStatusesAndAttrValueShapes(t *testing.T) {
	schema := loadSchema(t, "signal")
	span := map[string]any{"traceId": "t-1", "name": "order", "start": "2026-10-15T00:07:26.138Z", "status": "incomplete", "links": []any{map[string]any{"traceId": "t-0", "spanId": "s-0"}}}
	valid := []map[string]any{
		signalDoc("event", map[string]any{"body": "{}"}),
		signalDoc("metric", map[string]any{"name": "orders.rows", "value": 3.5}),
		signalDoc("span", span),
	}
	valid[2]["id"], valid[2]["rev"] = "s-1", 2
	for i, doc := range valid {
		if err := schema.Validate(doc); err != nil {
			t.Fatalf("valid signal %d was rejected: %v", i, err)
		}
	}
	mutations := []struct {
		name  string
		value any
		path  []any
	}{
		{"tone outside the set", "purple", []any{"attrs", "tone"}},
		{"object attr value", map[string]any{}, []any{"attrs", "nested"}},
		{"span status outside the set", "done", []any{"span", "status"}},
		{"signal kind outside the set", "log", []any{"kind"}},
		{"time source outside the set", "browser", []any{"tsSource"}},
		{"unknown header field", 1, []any{"extra"}},
	}
	for _, m := range mutations {
		doc := signalDoc("span", span)
		doc["id"] = "s-1"
		set(t, doc, m.value, m.path...)
		if err := schema.Validate(doc); err == nil {
			t.Fatalf("%s passed", m.name)
		}
	}
}

package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const (
	fieldsBaseline = "response-fields.json"
	updateBaseline = "COCKPIT_UPDATE_API_BASELINE"
)

var marshaler = reflect.TypeFor[json.Marshaler]()

var responseTypes = map[string]reflect.Type{
	"RunItem":     reflect.TypeFor[RunItem](),
	"RunDetail":   reflect.TypeFor[RunDetail](),
	"HubList":     reflect.TypeFor[HubList](),
	"ReportView":  reflect.TypeFor[ReportView](),
	"ExecView":    reflect.TypeFor[ExecView](),
	"FlowView":    reflect.TypeFor[FlowView](),
	"SourceView":  reflect.TypeFor[SourceView](),
	"SignalsView": reflect.TypeFor[SignalsView](),
	"KitView":     reflect.TypeFor[KitView](),
	"Problem":     reflect.TypeFor[Problem](),
}

func custom(t reflect.Type) bool {
	return t.Implements(marshaler) || reflect.PointerTo(t).Implements(marshaler)
}

func wireKind(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		return wireKind(t.Elem()) + "|null"
	}
	if custom(t) {
		if t == reflect.TypeFor[json.RawMessage]() {
			return "any"
		}
		return "custom " + t.String()
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice, reflect.Array:
		return "array of " + wireKind(t.Elem())
	case reflect.Map:
		return "map of " + wireKind(t.Elem())
	case reflect.Struct:
		return "object"
	}
	return "any"
}

func responseFields(t reflect.Type) map[string]string {
	out := map[string]string{}
	walkFields(t, "", out, map[reflect.Type]bool{})
	return out
}

func walkFields(t reflect.Type, prefix string, out map[string]string, stack map[reflect.Type]bool) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
		switch t.Kind() {
		case reflect.Slice, reflect.Array:
			prefix += "[]"
		case reflect.Map:
			prefix += "{}"
		}
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || custom(t) || stack[t] {
		return
	}
	stack[t] = true
	defer delete(stack, t)
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		name, opts, _ := strings.Cut(tag, ",")
		if tag == "-" || (!f.IsExported() && !f.Anonymous) {
			continue
		}
		if f.Anonymous && name == "" {
			walkFields(f.Type, prefix, out, stack)
			continue
		}
		if name == "" {
			name = f.Name
		}
		path := strings.TrimPrefix(prefix+"."+name, ".")
		kind := wireKind(f.Type)
		if slices.Contains(strings.Split(opts, ","), "omitempty") || slices.Contains(strings.Split(opts, ","), "omitzero") {
			kind += ", optional"
		}
		out[path] = kind
		walkFields(f.Type, path, out, stack)
	}
}

func currentFields() map[string]map[string]string {
	out := map[string]map[string]string{}
	for name, typ := range responseTypes {
		out[name] = responseFields(typ)
	}
	return out
}

func fieldBreaks(baseline, current map[string]map[string]string) (breaks, added []string) {
	for view, fields := range baseline {
		now, ok := current[view]
		if !ok {
			breaks = append(breaks, view+" is no longer a response shape")
			continue
		}
		for path, kind := range fields {
			switch got, ok := now[path]; {
			case !ok:
				breaks = append(breaks, view+"."+path+" disappeared (was "+kind+")")
			case got != kind:
				breaks = append(breaks, view+"."+path+" changed from "+kind+" to "+got)
			}
		}
		for path := range now {
			if _, ok := fields[path]; !ok {
				added = append(added, view+"."+path)
			}
		}
	}
	slices.Sort(breaks)
	slices.Sort(added)
	return breaks, added
}

func TestEveryResponseFieldInTheBaselineKeepsItsNameAndTypeAndNewFieldsPass(t *testing.T) {
	path := filepath.Join("testdata", fieldsBaseline)
	current := currentFields()
	if os.Getenv(updateBaseline) == "1" {
		data, err := json.MarshalIndent(current, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var baseline map[string]map[string]string
	if err := json.Unmarshal(data, &baseline); err != nil {
		t.Fatal(err)
	}
	fields := 0
	for view := range responseTypes {
		if len(baseline[view]) == 0 {
			t.Errorf("the baseline has no fields for %s", view)
		}
		fields += len(baseline[view])
	}
	if len(baseline) != len(responseTypes) || fields < 100 {
		t.Fatalf("the baseline holds %d shapes and %d fields", len(baseline), fields)
	}
	breaks, added := fieldBreaks(baseline, current)
	for _, b := range breaks {
		t.Errorf("%s; the API only adds fields (to change it on purpose, see docs/api.md)", b)
	}
	if len(added) > 0 {
		t.Logf("fields not yet in the baseline: %s", strings.Join(added, ", "))
	}
}

func TestADroppedOrRetypedFieldBreaksTheBaselineAndAnAddedFieldDoesNot(t *testing.T) {
	baseline := map[string]map[string]string{"RunItem": {"id": "string", "slot": "integer|null, optional", "redaction.files": "array of string, optional"}}
	cases := []struct {
		name    string
		current map[string]string
		breaks  int
		added   int
	}{
		{"unchanged", map[string]string{"id": "string", "slot": "integer|null, optional", "redaction.files": "array of string, optional"}, 0, 0},
		{"one added", map[string]string{"id": "string", "slot": "integer|null, optional", "redaction.files": "array of string, optional", "extra": "boolean"}, 0, 1},
		{"one dropped", map[string]string{"id": "string", "slot": "integer|null, optional"}, 1, 0},
		{"one retyped", map[string]string{"id": "integer", "slot": "integer|null, optional", "redaction.files": "array of string, optional"}, 1, 0},
		{"one made optional", map[string]string{"id": "string, optional", "slot": "integer|null, optional", "redaction.files": "array of string, optional"}, 1, 0},
	}
	for _, c := range cases {
		breaks, added := fieldBreaks(baseline, map[string]map[string]string{"RunItem": c.current})
		if len(breaks) != c.breaks || len(added) != c.added {
			t.Errorf("%s: breaks %q added %q", c.name, breaks, added)
		}
	}
	if breaks, _ := fieldBreaks(baseline, map[string]map[string]string{}); len(breaks) != 1 {
		t.Errorf("a removed shape: %q", breaks)
	}
	if len(cases) != 5 {
		t.Fatal("case count")
	}
	got := responseFields(reflect.TypeFor[RunDetail]())
	for path, want := range map[string]string{"id": "string", "slot": "integer|null, optional", "verdict": "custom lifecycle.Verdict", "services[].hostPort": "integer", "redaction.files": "array of string"} {
		if !strings.HasPrefix(got[path], want) {
			t.Errorf("RunDetail %s is %q, want %q", path, got[path], want)
		}
	}
}

package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

type sampleBase struct {
	Base string `json:"base"`
}

type sample struct {
	sampleBase
	Plain    string              `json:"plain"`
	Maybe    *int                `json:"maybe"`
	Omitted  *int                `json:"omitted,omitempty"`
	Zero     int                 `json:"zero,omitzero"`
	Skipped  string              `json:"-"`
	Raw      json.RawMessage     `json:"raw"`
	Bytes    []byte              `json:"bytes"`
	Nested   map[string][]string `json:"nested"`
	Odd      bool                `json:"odd-key"`
	Untagged float64
	hidden   string
}

type selfMarshal string

func (selfMarshal) MarshalJSON() ([]byte, error) { return []byte(`"x"`), nil }

type Event struct{}

func TestRenderingRulesForOptionalNullableEmbeddedAndOpaqueFields(t *testing.T) {
	_, text := generated(t, Config{Roots: []reflect.Type{reflect.TypeFor[sample]()}})
	want := "export interface sample extends sampleBase {\n" +
		"\tplain: string;\n" +
		"\tmaybe: number | null;\n" +
		"\tomitted?: number;\n" +
		"\tzero?: number;\n" +
		"\traw: unknown;\n" +
		"\tbytes: string;\n" +
		"\tnested: Record<string, string[]>;\n" +
		"\t\"odd-key\": boolean;\n" +
		"\tUntagged: number;\n" +
		"}\n" +
		"export interface sampleBase {\n\tbase: string;\n}\n"
	if text != want {
		t.Fatalf("got\n%s\nwant\n%s", text, want)
	}
	if strings.Contains(text, "Skipped") || strings.Contains(text, "hidden") {
		t.Fatalf("skipped fields rendered:\n%s", text)
	}
}

func TestEnumsListEveryTypedConstantAndNullZeroTypesDropTheEmptyValue(t *testing.T) {
	g, _ := generated(t, config)
	phase := g.decls["Phase"]
	for _, v := range []string{`"queued"`, `"testing"`, `"done"`} {
		if !strings.Contains(phase, v) {
			t.Errorf("Phase lacks %s: %s", v, phase)
		}
	}
	if strings.Contains(phase, `"absent"`) || strings.Count(phase, " | ") != 10 {
		t.Errorf("Phase holds another type's value or the wrong count: %s", phase)
	}
	if verdict := g.decls["Verdict"]; verdict != `export type Verdict = "passed" | "failed" | "error" | "aborted";` {
		t.Errorf("Verdict %s", verdict)
	}
	if !strings.Contains(g.decls["RunItem"], "\tverdict: Verdict | null;") {
		t.Errorf("RunItem verdict is not nullable:\n%s", g.decls["RunItem"])
	}
	if destroy := g.decls["DestroyReason"]; destroy != `export type DestroyReason = "" | "person" | "pool";` {
		t.Errorf("canary: DestroyReason keeps its empty value, got %s", destroy)
	}
	if state := g.decls["CollectorState"]; !strings.Contains(state, "\tstate: State;") || g.decls["State"] != `export type State = "armed" | "backoff" | "stopped" | "report-only";` {
		t.Errorf("CollectorState state %s, State %s", state, g.decls["State"])
	}
	if source := g.decls["SourceView"]; !strings.Contains(source, "\tsource: string;") {
		t.Errorf("canary: an untyped string field stays string, got %s", source)
	}
}

func TestSelfMarshallingTypesAndNameCollisionsAreRefused(t *testing.T) {
	type holder struct {
		Value selfMarshal `json:"value"`
	}
	cases := []struct {
		roots []reflect.Type
		want  string
	}{
		{[]reflect.Type{reflect.TypeFor[holder]()}, "marshals itself"},
		{[]reflect.Type{reflect.TypeFor[lifecycle.Event](), reflect.TypeFor[Event]()}, "both render as Event"},
		{[]reflect.Type{reflect.TypeFor[lifecycle.Verdict]()}, "marshals itself"},
	}
	ran := 0
	for _, c := range cases {
		_, err := Render(repoRoot(t), Config{Roots: c.roots})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v", c.roots, err)
		}
		ran++
	}
	if ran != 3 {
		t.Fatalf("ran %d cases", ran)
	}
	if _, err := Render(repoRoot(t), Config{Roots: []reflect.Type{reflect.TypeFor[api.RunItem](), reflect.TypeFor[lifecycle.Event]()}, NullZero: config.NullZero}); err != nil {
		t.Fatalf("canary: a clean config is refused: %v", err)
	}
}

func TestTsgenMutationRecordMatchesTestFiles(t *testing.T) {
	porttest.AssertMutationRecordAcross(t, []string{"render_test.go", "rules_test.go"}, tsgenMutations)
}

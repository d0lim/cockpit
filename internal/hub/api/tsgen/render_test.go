package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var tsgenMutations = []porttest.Mutation{
	{"omitempty fields are optional", "drop the omit check in iface so every field is required", "red: RunItem.mark loses its question mark and the schema comparison names it", "TestEveryInterfaceHasThePropertiesAndRequiredFieldsOfTheJSONSchemaOfItsStruct"},
	{"pointer fields without omitempty are nullable", "never append | null to pointer fields", "red: the sample maybe field renders as number", "TestRenderingRulesForOptionalNullableEmbeddedAndOpaqueFields"},
	{"embedded structs become extends", "skip anonymous fields", "red: RunDetail loses extends RunItem and the schema comparison names id", "TestEveryInterfaceHasThePropertiesAndRequiredFieldsOfTheJSONSchemaOfItsStruct"},
	{"enum values come from typed constants", "return no values from enumSource.values", "red: Phase renders as string", "TestEnumsListEveryTypedConstantAndNullZeroTypesDropTheEmptyValue"},
	{"null-zero types are nullable", "drop the NullZero case in ref", "red: Verdict reaches the marshaler refusal", "TestEnumsListEveryTypedConstantAndNullZeroTypesDropTheEmptyValue"},
	{"self-marshalling types are refused", "delete the json.Marshaler case in ref", "red: selfMarshal renders as a string type", "TestSelfMarshallingTypesAndNameCollisionsAreRefused"},
	{"name collisions are refused", "claim returns false without comparing owners", "red: the second Event is silently dropped", "TestSelfMarshallingTypesAndNameCollisionsAreRefused"},
	{"collector state is a typed union", "type api.CollectorState.State as a plain string", "red: State is missing and the field renders as string", "TestEnumsListEveryTypedConstantAndNullZeroTypesDropTheEmptyValue"},
	{"the committed file is the generator output", "edit web/shared/api.ts by hand", "red: the file differs from Render", "TestCommittedAPITypesAreTheGeneratorOutput"},
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
}

func generated(t *testing.T, cfg Config) (*generator, string) {
	t.Helper()
	g, err := newGenerator(repoRoot(t), cfg)
	if err != nil {
		t.Fatal(err)
	}
	text, err := g.render()
	if err != nil {
		t.Fatal(err)
	}
	return g, text
}

func TestCommittedAPITypesAreTheGeneratorOutput(t *testing.T) {
	_, want := generated(t, config)
	have, err := os.ReadFile(filepath.Join(repoRoot(t), "web", "shared", "api.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != want {
		t.Fatal("web/shared/api.ts differs from the generator output; run go generate ./internal/hub/api")
	}
	if !strings.Contains(want, "export interface RunDetail extends RunItem {") {
		t.Fatalf("canary: RunDetail is missing from\n%s", want)
	}
}

func schemaOf(t *testing.T, typ reflect.Type) *jsonschema.Schema {
	t.Helper()
	s, err := jsonschema.ForType(typ, &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[lifecycle.Verdict](): {Types: []string{"string", "null"}},
		reflect.TypeFor[json.RawMessage]():   {},
		reflect.TypeFor[map[string]any]():    {Type: "object"},
	}})
	if err != nil {
		t.Fatalf("%s: %v", typ, err)
	}
	return s
}

func TestEveryInterfaceHasThePropertiesAndRequiredFieldsOfTheJSONSchemaOfItsStruct(t *testing.T) {
	g, _ := generated(t, config)
	compared := 0
	for typ, fields := range g.fields {
		s := schemaOf(t, typ)
		var names, required []string
		for _, f := range fields {
			names = append(names, f.Name)
			if !f.Optional {
				required = append(required, f.Name)
			}
		}
		var props []string
		for k := range s.Properties {
			props = append(props, k)
		}
		want := slices.Clone(s.Required)
		for _, list := range [][]string{names, required, props, want} {
			slices.Sort(list)
		}
		if !slices.Equal(names, props) || !slices.Equal(required, want) {
			t.Errorf("%s: fields %v required %v; schema properties %v required %v", typ, names, required, props, want)
		}
		compared++
	}
	if compared != 40 {
		t.Fatalf("compared %d interfaces", compared)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/google/jsonschema-go/jsonschema"
)

const schemaDialect = "https://json-schema.org/draft/2020-12/schema"

var attrValueTypes = []string{"string", "number", "boolean"}

func typeSchemas() (map[reflect.Type]*jsonschema.Schema, error) {
	types := map[reflect.Type]*jsonschema.Schema{}
	for _, e := range contract.Enums() {
		kind := "integer"
		if e.Type.Kind() == reflect.String {
			kind = "string"
		}
		types[e.Type] = &jsonschema.Schema{Type: kind, Enum: slices.Clone(e.Values)}
	}
	for _, p := range contract.Patterns() {
		types[p.Type] = &jsonschema.Schema{Type: "string", Pattern: p.Pattern}
	}
	reserved, err := jsonschema.ForType(reflect.TypeFor[contract.ReservedAttrs](), &jsonschema.ForOptions{TypeSchemas: types})
	if err != nil {
		return nil, err
	}
	types[reflect.TypeFor[contract.Attrs]()] = &jsonschema.Schema{
		Type:                 "object",
		Properties:           reserved.Properties,
		AdditionalProperties: &jsonschema.Schema{Types: slices.Clone(attrValueTypes)},
	}
	return types, nil
}

func dropNull(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if len(s.Types) > 0 {
		kept := slices.DeleteFunc(slices.Clone(s.Types), func(t string) bool { return t == "null" })
		if len(kept) == 1 {
			s.Type, s.Types = kept[0], nil
		} else {
			s.Types = kept
		}
	}
	for _, p := range s.Properties {
		dropNull(p)
	}
	dropNull(s.Items)
	dropNull(s.AdditionalProperties)
}

func buildSchemas() (map[string][]byte, error) {
	types, err := typeSchemas()
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, r := range contract.Roots() {
		s, err := jsonschema.ForType(r.Type, &jsonschema.ForOptions{TypeSchemas: types})
		if err != nil {
			return nil, fmt.Errorf("schema %s: %w", r.Name, err)
		}
		dropNull(s)
		s.Schema = schemaDialect
		s.Title = r.Name
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(s); err != nil {
			return nil, fmt.Errorf("schema %s: %w", r.Name, err)
		}
		out[r.Name+".json"] = buf.Bytes()
	}
	return out, nil
}

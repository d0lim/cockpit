package main

import (
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func sampleSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
		"env": {Type: "object", Properties: map[string]*jsonschema.Schema{"image": {Type: "string"}}},
		"ports": {Type: "array", Items: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
			"name": {Type: "string"},
		}}},
	}}
}

func TestEveryDeclarationFieldGetsItsDocIncludingFieldsOfArrayItems(t *testing.T) {
	s := sampleSchema()
	docs := map[string]string{"": "R.", "env": "E.", "env.image": "I.", "ports": "P.", "ports.name": "N."}
	if err := describeAll(s, docs); err != nil {
		t.Fatal(err)
	}
	if s.Description != "R." || s.Properties["env"].Properties["image"].Description != "I." || s.Properties["ports"].Items.Properties["name"].Description != "N." {
		t.Fatalf("descriptions not set: %+v", s.Properties["ports"].Items.Properties["name"])
	}
}

func TestAFieldWithoutADocOrADocWithoutAFieldFailsGeneration(t *testing.T) {
	err := describeAll(sampleSchema(), map[string]string{"": "R.", "env": "E.", "ports": "P.", "ports.name": "N."})
	if err == nil || !strings.Contains(err.Error(), "env.image") {
		t.Fatalf("missing doc: err %v", err)
	}
	err = describeAll(sampleSchema(), map[string]string{"": "R.", "env": "E.", "env.image": "I.", "ports": "P.", "ports.name": "N.", "ports.gone": "G."})
	if err == nil || !strings.Contains(err.Error(), "ports.gone") {
		t.Fatalf("unused doc: err %v", err)
	}
}

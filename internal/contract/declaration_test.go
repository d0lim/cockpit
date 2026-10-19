package contract

import (
	"path/filepath"
	"strings"
	"testing"
)

func declarationSample(t *testing.T) map[string]any {
	t.Helper()
	return readDoc(t, filepath.Join("testdata", "declaration.json"))
}

func TestDeclarationSamplePassesTheDeclarationSchema(t *testing.T) {
	if err := loadSchema(t, "declaration").Validate(declarationSample(t)); err != nil {
		t.Fatal(err)
	}
}

func TestDeclarationWithoutSecretsSectionFailsSchemaValidation(t *testing.T) {
	schema := loadSchema(t, "declaration")
	doc := declarationSample(t)
	if err := schema.Validate(doc); err != nil {
		t.Fatalf("original did not pass: %v", err)
	}
	delete(doc, "secrets")
	err := schema.Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "secrets") {
		t.Fatalf("declaration without secrets was not rejected: %v", err)
	}
}

func TestDeclarationWithStringHostApiFailsSchemaValidation(t *testing.T) {
	schema := loadSchema(t, "declaration")
	doc := declarationSample(t)
	doc["hostApi"] = "1"
	if err := schema.Validate(doc); err == nil {
		t.Fatal("string hostApi passed")
	}
	doc["hostApi"] = 2
	if err := schema.Validate(doc); err != nil {
		t.Fatalf("integer hostApi was rejected: %v", err)
	}
}

type fieldCase struct {
	path   []any
	accept []any
	reject []any
}

func checkFields(t *testing.T, cases []fieldCase) (accepted, rejected int) {
	t.Helper()
	schema := loadSchema(t, "declaration")
	for _, c := range cases {
		for _, value := range c.accept {
			doc := declarationSample(t)
			set(t, doc, value, c.path...)
			if err := schema.Validate(doc); err != nil {
				t.Errorf("%v = %q was rejected: %v", c.path, value, err)
			}
			accepted++
		}
		for _, value := range c.reject {
			doc := declarationSample(t)
			set(t, doc, value, c.path...)
			if err := schema.Validate(doc); err == nil {
				t.Errorf("%v = %q passed", c.path, value)
			}
			rejected++
		}
	}
	return accepted, rejected
}

func TestDeclarationPathFieldsRejectPathsOutsideTheCommitRootDotSegmentsAndEmptySegments(t *testing.T) {
	accept := []any{"app", ".env", "a/b.c", "a/.hidden/c", "...", "x..y", "app/.secrets.json"}
	reject := []any{"", "/abs", "../x", "./x", "..", ".", "a/..", "a/../b", "a/./b", "a//b", "a/"}
	paths := [][]any{
		{"compose", "files", 0},
		{"compose", "projectDirectory"},
		{"inputs", "dirty", 0},
		{"inputs", "runnerSources", 0, "path"},
		{"images", 0, "inputs", 0},
		{"env", "outputs", 0},
		{"secrets", "sources", 0, "path"},
		{"scripts", "dir"},
		{"scripts", "suites"},
		{"scripts", "collectors"},
	}
	var cases []fieldCase
	for _, p := range paths {
		cases = append(cases, fieldCase{path: p, accept: accept, reject: reject})
	}
	accepted, rejected := checkFields(t, cases)
	if accepted != len(paths)*len(accept) || rejected != len(paths)*len(reject) {
		t.Fatalf("evaluated: accepted %d, rejected %d", accepted, rejected)
	}
}

func TestDeclarationNameFieldsAcceptOnlyLowercaseDigitsAndHyphensUpTo63Chars(t *testing.T) {
	longest := "a" + strings.Repeat("b", 62)
	accept := []any{"app", "a-1", "0x", longest}
	reject := []any{"", "App", "-a", "a_b", "a/b", "..", "a b", longest + "c"}
	paths := [][]any{
		{"ports", 0, "name"},
		{"inputs", "runnerSources", 0, "name"},
		{"databases", 0, "name"},
	}
	var cases []fieldCase
	for _, p := range paths {
		cases = append(cases, fieldCase{path: p, accept: accept, reject: reject})
	}
	accepted, rejected := checkFields(t, cases)
	if accepted != len(paths)*len(accept) || rejected != len(paths)*len(reject) {
		t.Fatalf("evaluated: accepted %d, rejected %d", accepted, rejected)
	}
}

func TestDeclarationEnvImageAcceptsOnlyDigestPinnedReferences(t *testing.T) {
	digest := "sha256:" + strings.Repeat("0123456789abcdef", 4)
	accepted, rejected := checkFields(t, []fieldCase{{
		path:   []any{"env", "image"},
		accept: []any{"alpine@" + digest, "registry.example:5000/tools/env:1@" + digest},
		reject: []any{"alpine:3.20", "alpine@sha256:abc", "alpine@" + strings.ToUpper(digest), "@" + digest},
	}})
	if accepted != 2 || rejected != 4 {
		t.Fatalf("evaluated: accepted %d, rejected %d", accepted, rejected)
	}
}

func TestDeclarationEnumFieldsAcceptOnlyTheirDefinedValues(t *testing.T) {
	accepted, rejected := checkFields(t, []fieldCase{
		{path: []any{"ports", 0, "open"}, accept: []any{"browser", "link", "copy"}, reject: []any{"window", ""}},
		{path: []any{"databases", 0, "driver"}, accept: []any{"mysql", "postgres"}, reject: []any{"sqlite"}},
		{path: []any{"secrets", "sources", 0, "format"}, accept: []any{"dotenv", "json", "pem"}, reject: []any{"xml"}},
		{path: []any{"images", 0, "key"}, accept: []any{"IMG_APP_2"}, reject: []any{"IMG_app", "APP"}},
	})
	if accepted != 9 || rejected != 6 {
		t.Fatalf("evaluated: accepted %d, rejected %d", accepted, rejected)
	}
}

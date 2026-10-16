package contract

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func readDoc(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return doc
}

func cloneDoc(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func encodeDoc(t *testing.T, doc any) []byte {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func loadSchema(t *testing.T, name string) *jsonschema.Resolved {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "schema", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("schema/%s.json: %v", name, err)
	}
	resolved, err := s.Resolve(nil)
	if err != nil {
		t.Fatalf("schema/%s.json: %v", name, err)
	}
	return resolved
}

func at(t *testing.T, doc any, path ...any) any {
	t.Helper()
	cur := doc
	for _, step := range path {
		switch key := step.(type) {
		case string:
			obj, ok := cur.(map[string]any)
			if !ok {
				t.Fatalf("%v: value before %q is not an object", path, key)
			}
			cur = obj[key]
		case int:
			list, ok := cur.([]any)
			if !ok || key >= len(list) {
				t.Fatalf("%v: value before %d is not an array or is too short", path, key)
			}
			cur = list[key]
		}
	}
	return cur
}

func set(t *testing.T, doc any, value any, path ...any) {
	t.Helper()
	parent := at(t, doc, path[:len(path)-1]...)
	switch key := path[len(path)-1].(type) {
	case string:
		parent.(map[string]any)[key] = value
	case int:
		parent.([]any)[key] = value
	}
}

func jsonNames(t reflect.Type) []string {
	var names []string
	for _, f := range reflect.VisibleFields(t) {
		tag := f.Tag.Get("json")
		if f.Anonymous || tag == "" || tag == "-" {
			continue
		}
		names = append(names, strings.Split(tag, ",")[0])
	}
	slices.Sort(names)
	return names
}

func sorted(names ...string) []string {
	out := slices.Clone(names)
	slices.Sort(out)
	return out
}

func command(t *testing.T, dir string, env []string, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

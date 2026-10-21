package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

func describe(s *jsonschema.Schema, path string, docs map[string]string, used map[string]bool) []string {
	if s == nil {
		return nil
	}
	var missing []string
	if doc, ok := docs[path]; ok {
		s.Description = doc
		used[path] = true
	} else {
		missing = append(missing, fieldName(path))
	}
	node := s
	if node.Items != nil {
		node = node.Items
	}
	for name, p := range node.Properties {
		missing = append(missing, describe(p, strings.TrimPrefix(path+"."+name, "."), docs, used)...)
	}
	return missing
}

func fieldName(path string) string {
	if path == "" {
		return "(root)"
	}
	return path
}

func describeAll(s *jsonschema.Schema, docs map[string]string) error {
	used := map[string]bool{}
	missing := describe(s, "", docs, used)
	if len(missing) > 0 {
		slices.Sort(missing)
		return fmt.Errorf("declaration fields without a doc: %s", strings.Join(missing, ", "))
	}
	return unusedDocs(docs, used)
}

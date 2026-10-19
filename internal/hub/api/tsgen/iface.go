package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

var identifier = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

func key(name string) string {
	if identifier.MatchString(name) {
		return name
	}
	raw, _ := json.Marshal(name)
	return string(raw)
}

func (g *generator) iface(t reflect.Type) (string, error) {
	var parents, lines []string
	var flat []Field
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" {
			if f.Type.Kind() != reflect.Struct {
				return "", fmt.Errorf("%s embeds %s, which is not a struct", t, f.Type)
			}
			parent, err := g.ref(f.Type)
			if err != nil {
				return "", err
			}
			parents = append(parents, parent)
			flat = append(flat, g.fields[f.Type]...)
			continue
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		options := strings.Split(opts, ",")
		omit := slices.Contains(options, "omitempty") || slices.Contains(options, "omitzero")
		typ, err := g.ref(f.Type)
		if err != nil {
			return "", fmt.Errorf("%s.%s: %w", t.Name(), f.Name, err)
		}
		mark := ""
		switch {
		case omit:
			mark = "?"
		case f.Type.Kind() == reflect.Pointer && !strings.HasSuffix(typ, " | null"):
			typ += " | null"
		}
		lines = append(lines, "\t"+key(name)+mark+": "+typ+";")
		flat = append(flat, Field{Name: name, Optional: omit})
	}
	g.fields[t] = flat
	head := "export interface " + t.Name()
	if len(parents) > 0 {
		head += " extends " + strings.Join(parents, ", ")
	}
	if len(lines) == 0 {
		return head + " {}", nil
	}
	return head + " {\n" + strings.Join(lines, "\n") + "\n}", nil
}

package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
)

type Field struct {
	Name     string
	Optional bool
}

type Config struct {
	Roots    []reflect.Type
	NullZero []reflect.Type
}

type generator struct {
	cfg    Config
	enums  *enumSource
	decls  map[string]string
	owners map[string]reflect.Type
	fields map[reflect.Type][]Field
}

var (
	rawMessage = reflect.TypeFor[json.RawMessage]()
	timeType   = reflect.TypeFor[time.Time]()
	marshaler  = reflect.TypeFor[json.Marshaler]()
)

func newGenerator(root string, cfg Config) (*generator, error) {
	enums, err := newEnumSource(root)
	if err != nil {
		return nil, err
	}
	return &generator{cfg: cfg, enums: enums, decls: map[string]string{}, owners: map[string]reflect.Type{}, fields: map[reflect.Type][]Field{}}, nil
}

func Render(root string, cfg Config) (string, error) {
	g, err := newGenerator(root, cfg)
	if err != nil {
		return "", err
	}
	return g.render()
}

func (g *generator) render() (string, error) {
	for _, t := range g.cfg.Roots {
		if _, err := g.ref(t); err != nil {
			return "", err
		}
	}
	names := make([]string, 0, len(g.decls))
	for name := range g.decls {
		names = append(names, name)
	}
	slices.Sort(names)
	var b strings.Builder
	for _, name := range names {
		b.WriteString(g.decls[name] + "\n")
	}
	return b.String(), nil
}

func (g *generator) claim(t reflect.Type) (bool, error) {
	name := t.Name()
	if owner, ok := g.owners[name]; ok {
		if owner != t {
			return false, fmt.Errorf("%s and %s both render as %s", owner, t, name)
		}
		return false, nil
	}
	g.owners[name] = t
	return true, nil
}

func union(text string) string {
	if strings.Contains(text, " | ") {
		return "(" + text + ")"
	}
	return text
}

func (g *generator) ref(t reflect.Type) (string, error) {
	switch {
	case t == rawMessage:
		return "unknown", nil
	case t == timeType:
		return "string", nil
	case slices.Contains(g.cfg.NullZero, t):
		name, err := g.named(t, g.enum)
		return name + " | null", err
	case t.Implements(marshaler) || reflect.PointerTo(t).Implements(marshaler):
		return "", fmt.Errorf("%s marshals itself; list it in Config.NullZero or give it a TypeScript form", t)
	}
	switch t.Kind() {
	case reflect.Pointer:
		return g.ref(t.Elem())
	case reflect.Interface:
		return "unknown", nil
	case reflect.Bool:
		return "boolean", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return "number", nil
	case reflect.String:
		if t.PkgPath() == "" {
			return "string", nil
		}
		return g.named(t, g.enum)
	case reflect.Slice, reflect.Map:
		if t.Name() != "" {
			return g.named(t, g.alias)
		}
		return g.container(t)
	case reflect.Struct:
		return g.named(t, g.iface)
	}
	return "", fmt.Errorf("%s has kind %s, which has no TypeScript form", t, t.Kind())
}

func (g *generator) container(t reflect.Type) (string, error) {
	if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
		return "string", nil
	}
	elem, err := g.ref(t.Elem())
	if err != nil {
		return "", err
	}
	if t.Kind() == reflect.Slice {
		return union(elem) + "[]", nil
	}
	if t.Key().Kind() != reflect.String {
		return "", fmt.Errorf("%s has a non-string key", t)
	}
	return "Record<string, " + elem + ">", nil
}

func (g *generator) named(t reflect.Type, build func(reflect.Type) (string, error)) (string, error) {
	fresh, err := g.claim(t)
	if err != nil || !fresh {
		return t.Name(), err
	}
	g.decls[t.Name()] = ""
	text, err := build(t)
	if err != nil {
		return "", err
	}
	g.decls[t.Name()] = text
	return t.Name(), nil
}

func (g *generator) alias(t reflect.Type) (string, error) {
	body, err := g.container(t)
	return "export type " + t.Name() + " = " + body + ";", err
}

func (g *generator) enum(t reflect.Type) (string, error) {
	values, err := g.enums.values(t)
	if err != nil {
		return "", err
	}
	if slices.Contains(g.cfg.NullZero, t) {
		values = slices.DeleteFunc(values, func(v string) bool { return v == "" })
	}
	if len(values) == 0 {
		return "export type " + t.Name() + " = string;", nil
	}
	literals := make([]string, len(values))
	for i, v := range values {
		raw, _ := json.Marshal(v)
		literals[i] = string(raw)
	}
	return "export type " + t.Name() + " = " + strings.Join(literals, " | ") + ";", nil
}

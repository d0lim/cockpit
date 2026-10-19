package main

import (
	"fmt"
	"go/constant"
	"go/types"
	"reflect"
	"slices"

	"golang.org/x/tools/go/packages"
)

type enumSource struct {
	root  string
	cache map[string]*types.Package
}

func newEnumSource(root string) (*enumSource, error) {
	return &enumSource{root: root, cache: map[string]*types.Package{}}, nil
}

func (e *enumSource) pkg(path string) (*types.Package, error) {
	if p, ok := e.cache[path]; ok {
		return p, nil
	}
	pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedName | packages.NeedTypes, Dir: e.root}, path)
	if err != nil {
		return nil, err
	}
	if len(pkgs) != 1 || len(pkgs[0].Errors) > 0 || pkgs[0].Types == nil {
		return nil, fmt.Errorf("package %s did not load: %v", path, pkgs)
	}
	e.cache[path] = pkgs[0].Types
	return pkgs[0].Types, nil
}

func (e *enumSource) values(t reflect.Type) ([]string, error) {
	p, err := e.pkg(t.PkgPath())
	if err != nil {
		return nil, err
	}
	named := p.Scope().Lookup(t.Name())
	if named == nil {
		return nil, fmt.Errorf("%s is not declared in %s", t.Name(), t.PkgPath())
	}
	type entry struct {
		pos   int
		value string
	}
	var found []entry
	for _, name := range p.Scope().Names() {
		c, ok := p.Scope().Lookup(name).(*types.Const)
		if !ok || !types.Identical(c.Type(), named.Type()) || c.Val().Kind() != constant.String {
			continue
		}
		found = append(found, entry{pos: int(c.Pos()), value: constant.StringVal(c.Val())})
	}
	slices.SortFunc(found, func(a, b entry) int { return a.pos - b.pos })
	var out []string
	for _, f := range found {
		if !slices.Contains(out, f.value) {
			out = append(out, f.value)
		}
	}
	return out, nil
}

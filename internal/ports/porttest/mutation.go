package porttest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

type Mutation [4]string

func (m Mutation) Control() string { return m[0] }

func (m Mutation) Disable() string { return m[1] }

func (m Mutation) Result() string { return m[2] }

func (m Mutation) Test() string { return m[3] }

func AssertMutationRecord(t *testing.T, file string, record []Mutation) {
	t.Helper()
	if len(record) == 0 {
		t.Fatal("the mutation record is empty")
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]bool{}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
			tests[fn.Name.Name] = true
		}
	}
	covered := map[string]bool{}
	for _, m := range record {
		if !tests[m.Test()] {
			t.Errorf("recorded test %q is not in %s", m.Test(), file)
		}
		if m.Control() == "" || m.Disable() == "" || !strings.HasPrefix(m.Result(), "red") {
			t.Errorf("record row %q must name the control, how to disable it, and a red result", m.Control())
		}
		covered[m.Test()] = true
	}
	for name := range tests {
		if !covered[name] && !strings.Contains(name, "MutationRecord") {
			t.Errorf("test %q is not in the mutation record", name)
		}
	}
}

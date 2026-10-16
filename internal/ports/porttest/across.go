package porttest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func AssertMutationRecordAcross(t *testing.T, files []string, record []Mutation) {
	t.Helper()
	if len(record) == 0 {
		t.Fatal("the mutation record is empty")
	}
	tests := map[string]bool{}
	for _, file := range files {
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
				tests[fn.Name.Name] = true
			}
		}
	}
	covered := map[string]bool{}
	for _, m := range record {
		if !tests[m.Test()] {
			t.Errorf("recorded test %q is not in %v", m.Test(), files)
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

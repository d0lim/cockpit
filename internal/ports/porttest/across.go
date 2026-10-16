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
		t.Fatal("변이 기록표가 비어 있다")
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
			t.Errorf("기록표의 시험 %q 이 %v 에 없다", m.Test(), files)
		}
		if m.Control() == "" || m.Disable() == "" || !strings.HasPrefix(m.Result(), "빨강") {
			t.Errorf("기록표 행 %q 은 확인 대상·무력화 방법·빨강 결과를 전부 가져야 한다", m.Control())
		}
		covered[m.Test()] = true
	}
	for name := range tests {
		if !covered[name] && !strings.Contains(name, "기록표") {
			t.Errorf("시험 %q 이 변이 기록표에 없다", name)
		}
	}
}

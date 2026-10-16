package engine

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

const (
	commitRoot = "testdata"
	scriptRoot = "testdata/adversarial"
)

type probe struct {
	site    Site
	records []string
	rows    []int
	planted map[string]string
	release chan struct{}
	gates   map[string]chan struct{}
	extra   map[string]any
	host    *Host
}

func newProbe(site Site) *probe {
	gates := map[string]chan struct{}{"a": make(chan struct{}), "b": make(chan struct{})}
	return &probe{site: site, planted: map[string]string{}, release: make(chan struct{}), gates: gates}
}

func (p *probe) foreign() string {
	if p.site == SiteRunner {
		return ModuleCollect
	}
	return ModuleFlow
}

func (p *probe) module(h *Host) (goja.Value, error) {
	p.host = h
	rt := h.Runtime()
	obj := rt.NewObject()
	planted := rt.NewObject()
	for k, v := range p.planted {
		if err := planted.Set(k, v); err != nil {
			return nil, err
		}
	}
	set := map[string]any{
		"record":  func(text string) { p.records = append(p.records, text) },
		"foreign": p.foreign(),
		"planted": planted,
		"maxRows": func(call goja.FunctionCall) goja.Value {
			var opts struct {
				MaxRows int `json:"maxRows"`
			}
			if err := h.DecodeArgs(call.Argument(0), &opts); err != nil {
				panic(h.Error(err))
			}
			p.rows = append(p.rows, opts.MaxRows)
			return goja.Undefined()
		},
		"wait": func(goja.FunctionCall) goja.Value {
			return h.Async(func(ctx context.Context) (any, error) {
				<-p.release
				return "late", nil
			})
		},
		"hold": func(call goja.FunctionCall) goja.Value {
			gate := p.gates[call.Argument(0).String()]
			return h.Async(func(context.Context) (any, error) {
				<-gate
				return "released", nil
			})
		},
		"releaseInOrder": func(goja.FunctionCall) goja.Value {
			close(p.gates["a"])
			time.AfterFunc(10*time.Millisecond, func() { close(p.gates["b"]) })
			return goja.Undefined()
		},
		"echo": func(goja.FunctionCall) goja.Value {
			return h.Async(func(context.Context) (any, error) {
				return echoResult{Status: 201, Items: []string{"a", "b"}}, nil
			})
		},
		"deny": func(goja.FunctionCall) goja.Value {
			return h.Async(func(context.Context) (any, error) {
				return nil, &PortError{Code: PortDenied, Message: "denied by policy ****"}
			})
		},
		"leak": func(goja.FunctionCall) goja.Value {
			return h.Async(func(context.Context) (any, error) {
				return nil, errors.New("dial tcp: password=PLANTED_GO_ERROR")
			})
		},
		"badArgs": func(goja.FunctionCall) goja.Value {
			panic(h.Error(&PortError{Code: PortBadArgs, Message: "maxRows must be a number"}))
		},
	}
	for k, v := range p.extra {
		set[k] = v
	}
	for k, v := range set {
		if err := obj.Set(k, v); err != nil {
			return nil, err
		}
	}
	return obj, nil
}

type echoResult struct {
	Status int      `json:"status"`
	Items  []string `json:"items"`
}

func (echoResult) Credential() string { return "PLANTED_GO_METHOD" }

func newEngine(t *testing.T, site Site, p *probe, deadline time.Duration) *Engine {
	t.Helper()
	modules := map[string]ModuleBuilder{}
	if p != nil {
		modules[site.module()] = p.module
	}
	eng, err := New(Options{Site: site, Budget: Budget{Deadline: deadline}, Modules: modules})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return eng
}

func bundleAt(t *testing.T, root, entry string, site Site) (Bundle, error) {
	t.Helper()
	return BuildBundle(context.Background(), BundleInput{Entry: filepath.Join(root, entry), Root: root, Site: site})
}

func mustBundle(t *testing.T, root, entry string, site Site) Bundle {
	t.Helper()
	b, err := bundleAt(t, root, entry, site)
	if err != nil {
		t.Fatalf("번들 %s: %v", entry, err)
	}
	return b
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, src := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func runSource(t *testing.T, site Site, p *probe, src string) error {
	t.Helper()
	root := writeTree(t, map[string]string{"entry.ts": src})
	return newEngine(t, site, p, 5*time.Second).Run(context.Background(), mustBundle(t, root, "entry.ts", site), "default")
}

func failureOf(t *testing.T, err error, kind FailureKind) *Failure {
	t.Helper()
	var f *Failure
	if !errors.As(err, &f) || f.Kind != kind {
		t.Fatalf("실패 종류 %s 를 기대했는데 %v", kind, err)
	}
	return f
}

type mutation struct {
	target  string
	disable string
	result  string
	test    string
}

func assertMutationRecord(t *testing.T, file string, record []mutation) {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]bool{}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") && !strings.Contains(fn.Name.Name, "변이_기록표") {
			tests[fn.Name.Name] = true
		}
	}
	if len(tests) == 0 || len(record) == 0 {
		t.Fatalf("%s: 시험 %d 개, 기록 %d 줄", file, len(tests), len(record))
	}
	listed := map[string]bool{}
	for _, m := range record {
		listed[m.test] = true
		if !tests[m.test] {
			t.Errorf("기록표의 %s 가 %s 에 없다", m.test, file)
		}
		if m.target == "" || m.disable == "" || !strings.HasPrefix(m.result, "빨강") {
			t.Errorf("기록이 비었거나 변이가 살아남았다: %+v", m)
		}
	}
	for name := range tests {
		if !listed[name] {
			t.Errorf("%s 의 %s 가 기록표에 없다", file, name)
		}
	}
}

func plant(t *testing.T, name, content string) (abs, rel string) {
	t.Helper()
	abs = filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rel, err = filepath.Rel(wd, abs)
	if err != nil || !strings.HasPrefix(rel, "..") {
		t.Fatalf("상대 경로 %q %v", rel, err)
	}
	return abs, rel
}

package engine

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
	"github.com/go-sourcemap/sourcemap"
)

const (
	bundleName      = "bundle.js"
	inlineMapPrefix = "//# sourceMappingURL=data:application/json;base64,"
	parseFailure    = "the engine cannot parse the bundle: "
)

var errSourceMapFile = errors.New("source maps are only read from inline data")

func noDiskSourceMap(string) ([]byte, error) {
	return nil, errSourceMapFile
}

func compileBundle(code string) (*goja.Program, error) {
	prg, err := goja.Parse(bundleName, code, parser.WithSourceMapLoader(noDiskSourceMap))
	if err != nil {
		return nil, err
	}
	return goja.CompileAST(prg, false)
}

func parseProblems(code string) []Problem {
	_, err := parser.ParseFile(nil, bundleName, code, 0, parser.WithDisableSourceMaps)
	if err == nil {
		return nil
	}
	var list parser.ErrorList
	if !errors.As(err, &list) || len(list) == 0 {
		return []Problem{{File: bundleName, Text: parseFailure + err.Error()}}
	}
	sm := inlineSourceMap(code)
	problems := make([]Problem, 0, len(list))
	for _, e := range list {
		p := Problem{File: bundleName, Line: e.Position.Line, Column: e.Position.Column, Text: parseFailure + e.Message}
		if sm != nil {
			if source, _, line, column, ok := sm.Source(e.Position.Line, e.Position.Column); ok {
				p.File, p.Line, p.Column = source, line, column
			}
		}
		problems = append(problems, p)
	}
	return problems
}

func inlineSourceMap(code string) *sourcemap.Consumer {
	i := strings.LastIndex(code, inlineMapPrefix)
	if i < 0 {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(code[i+len(inlineMapPrefix):]))
	if err != nil {
		return nil
	}
	sm, err := sourcemap.Parse(bundleName, raw)
	if err != nil {
		return nil
	}
	return sm
}

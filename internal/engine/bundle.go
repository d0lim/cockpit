package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

type BundleInput struct {
	Entry string
	Root  string
	Site  Site
}

type Bundle struct {
	Code string
}

type Problem struct {
	File   string
	Line   int
	Column int
	Text   string
}

type BundleError struct {
	Problems []Problem
}

func (e *BundleError) Error() string {
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = fmt.Sprintf("%s:%d:%d: %s", p.File, p.Line, p.Column, p.Text)
	}
	return "bundle rejected: " + strings.Join(parts, "; ")
}

const bundleGlobal = "__cockpit"

var unsupportedSyntax = map[string]bool{
	"async-generator":      false,
	"using":                false,
	"decorators":           false,
	"top-level-await":      false,
	"regexp-match-indices": false,
	"regexp-set-notation":  false,
}

func BuildBundle(ctx context.Context, in BundleInput) (Bundle, error) {
	if err := ctx.Err(); err != nil {
		return Bundle{}, err
	}
	root, err := filepath.EvalSymlinks(in.Root)
	if err == nil {
		root, err = filepath.Abs(root)
	}
	if err != nil {
		return Bundle{}, fmt.Errorf("bundle root: %w", err)
	}
	entry, err := filepath.EvalSymlinks(in.Entry)
	if err == nil {
		entry, err = filepath.Abs(entry)
	}
	if err != nil {
		return Bundle{}, fmt.Errorf("bundle entry: %w", err)
	}
	res := api.Build(api.BuildOptions{
		EntryPoints:    []string{entry},
		Bundle:         true,
		Write:          false,
		Format:         api.FormatIIFE,
		Platform:       api.PlatformNeutral,
		GlobalName:     bundleGlobal,
		Banner:         map[string]string{"js": "(function (require) {"},
		Footer:         map[string]string{"js": "return " + bundleGlobal + ";\n})"},
		Sourcemap:      api.SourceMapInline,
		SourcesContent: api.SourcesContentExclude,
		Outfile:        filepath.Join(root, bundleName),
		AbsWorkingDir:  root,
		TsconfigRaw:    "{}",
		Supported:      unsupportedSyntax,
		LogLevel:       api.LogLevelSilent,
		Plugins:        []api.Plugin{boundary{root: root, site: in.Site}.plugin()},
	})
	if len(res.Errors) > 0 {
		return Bundle{}, &BundleError{Problems: problemsOf(res.Errors)}
	}
	if len(res.OutputFiles) != 1 {
		return Bundle{}, fmt.Errorf("bundle produced %d files", len(res.OutputFiles))
	}
	code := string(res.OutputFiles[0].Contents)
	if problems := parseProblems(code); len(problems) > 0 {
		return Bundle{}, &BundleError{Problems: problems}
	}
	return Bundle{Code: code}, nil
}

func problemsOf(messages []api.Message) []Problem {
	problems := make([]Problem, 0, len(messages))
	for _, m := range messages {
		p := Problem{Text: m.Text}
		if m.Location != nil {
			p.File, p.Line, p.Column = m.Location.File, m.Location.Line, m.Location.Column
		}
		problems = append(problems, p)
	}
	return problems
}

type boundary struct {
	root string
	site Site
}

func (b boundary) plugin() api.Plugin {
	return api.Plugin{Name: "cockpit-boundary", Setup: func(build api.PluginBuild) {
		build.OnResolve(api.OnResolveOptions{Filter: ".*"}, b.resolve)
		build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "file"}, b.load)
	}}
}

func (b boundary) resolve(args api.OnResolveArgs) (api.OnResolveResult, error) {
	spec := args.Path
	switch {
	case args.Kind == api.ResolveEntryPoint:
		return api.OnResolveResult{}, nil
	case args.Kind == api.ResolveJSDynamicImport:
		return api.OnResolveResult{}, fmt.Errorf("dynamic import of %q is not allowed", spec)
	case strings.HasPrefix(spec, modulePrefix):
		if !b.site.allows(spec) {
			return api.OnResolveResult{}, fmt.Errorf("module %q is not available at the %s site", spec, b.site)
		}
		return api.OnResolveResult{Path: spec, External: true}, nil
	case isPath(spec):
		target := spec
		if !filepath.IsAbs(target) {
			target = filepath.Join(args.ResolveDir, target)
		}
		if !within(b.root, target) {
			return api.OnResolveResult{}, fmt.Errorf("%q is outside the boundary", spec)
		}
		return api.OnResolveResult{}, nil
	}
	return api.OnResolveResult{}, fmt.Errorf("bare specifier %q is not allowed", spec)
}

func (b boundary) load(args api.OnLoadArgs) (api.OnLoadResult, error) {
	real, err := filepath.EvalSymlinks(args.Path)
	if err != nil || !within(b.root, real) {
		return api.OnLoadResult{}, fmt.Errorf("%q resolves outside the boundary", filepath.Base(args.Path))
	}
	return api.OnLoadResult{}, nil
}

func isPath(spec string) bool {
	return spec == "." || spec == ".." || strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") || strings.HasPrefix(spec, "/")
}

func within(root, target string) bool {
	rel, err := filepath.Rel(root, realish(target))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func realish(p string) string {
	p = filepath.Clean(p)
	rest := ""
	for {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(p, rest)
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}

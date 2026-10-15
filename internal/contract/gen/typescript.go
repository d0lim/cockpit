package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/gzuidhof/tygo/tygo"
)

const contractPackage = "github.com/d0lim/cockpit/internal/contract"

var dataFiles = []string{"manifest.go", "result.go", "ledger.go", "context.go", "exec.go", "summary.go", "meta.go", "declaration.go", "signal.go"}

var (
	declStart     = regexp.MustCompile(`^export (?:interface|type) ([A-Za-z0-9_]+)`)
	exportedName  = regexp.MustCompile(`(?m)^export (?:interface|type|function) ([A-Za-z0-9_]+)`)
	inlineComment = regexp.MustCompile(`\s*/\*[^*]*\*/`)
	identifier    = regexp.MustCompile(`\b[A-Z][A-Za-z0-9_]*\b`)
)

func tygoOutput() (string, error) {
	dir, err := os.MkdirTemp("", "contract-tygo-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "types.ts")
	cfg := &tygo.Config{Packages: []*tygo.PackageConfig{{
		Path:             contractPackage,
		OutputPath:       out,
		IncludeFiles:     dataFiles,
		PreserveComments: "none",
		Indent:           "\t",
	}}}
	if err := tygo.New(cfg).Generate(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(out)
	return string(data), err
}

func parseDecls(ts string) map[string]string {
	decls := map[string]string{}
	var name string
	var lines []string
	flush := func() {
		if name != "" {
			decls[name] = strings.TrimRight(strings.Join(lines, "\n"), "\n")
		}
		name, lines = "", nil
	}
	for _, line := range strings.Split(inlineComment.ReplaceAllString(ts, ""), "\n") {
		if strings.HasPrefix(line, "export ") || strings.HasPrefix(line, "//") {
			flush()
			if m := declStart.FindStringSubmatch(line); m != nil {
				name, lines = m[1], []string{line}
			}
			continue
		}
		if name != "" {
			lines = append(lines, line)
		}
	}
	flush()
	return decls
}

func dataDecls() (map[string]string, error) {
	ts, err := tygoOutput()
	if err != nil {
		return nil, err
	}
	decls := parseDecls(ts)
	for _, e := range contract.Enums() {
		literals := make([]string, len(e.Values))
		for i, v := range e.Values {
			b, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			literals[i] = string(b)
		}
		decls[e.Type.Name()] = fmt.Sprintf("export type %s = %s;", e.Type.Name(), strings.Join(literals, " | "))
	}
	decls["AttrValue"] = fmt.Sprintf("export type AttrValue = %s;", strings.Join(attrValueTypes, " | "))
	decls["Attrs"] = "export type Attrs = ReservedAttrs & { [key: string]: AttrValue };"
	return decls, nil
}

func closure(roots []string, extra []string, decls map[string]string) ([]string, error) {
	seen := map[string]bool{}
	var queue []string
	visit := func(name string) {
		if _, ok := decls[name]; ok && !seen[name] {
			seen[name] = true
			queue = append(queue, name)
		}
	}
	for _, r := range roots {
		if _, ok := decls[r]; !ok {
			return nil, fmt.Errorf("module root %s is not a contract type", r)
		}
		visit(r)
	}
	for _, text := range extra {
		for _, id := range identifier.FindAllString(text, -1) {
			visit(id)
		}
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		for _, id := range identifier.FindAllString(decls[name], -1) {
			visit(id)
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)
	return names, nil
}

func indent(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

func renderModule(m contract.Module, decls map[string]string) (string, error) {
	var members []string
	for _, member := range m.Members {
		members = append(members, member.TS)
	}
	extra := append(slices.Clone(m.Decls), members...)
	names, err := closure(m.Types, extra, decls)
	if err != nil {
		return "", fmt.Errorf("%s: %w", m.Name, err)
	}
	for _, d := range m.Decls {
		for _, match := range exportedName.FindAllStringSubmatch(d, -1) {
			if slices.Contains(names, match[1]) || match[1] == m.Ctx {
				return "", fmt.Errorf("%s: %s is declared twice", m.Name, match[1])
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "declare module %q {\n", m.Name)
	for _, name := range names {
		b.WriteString(indent(decls[name], "\t") + "\n")
	}
	for _, d := range m.Decls {
		b.WriteString(indent(d, "\t") + "\n")
	}
	fmt.Fprintf(&b, "\texport interface %s {\n", m.Ctx)
	for _, member := range members {
		b.WriteString("\t\t" + member + "\n")
	}
	b.WriteString("\t}\n}\n")
	return b.String(), nil
}

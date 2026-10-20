package main

import (
	"bytes"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

const suffix = "_templ.go"

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: stripcomments <dir>")
		os.Exit(2)
	}
	n, err := stripDir(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if n == 0 {
		fmt.Fprintln(os.Stderr, "no "+suffix+" files in "+os.Args[1])
		os.Exit(1)
	}
}

func stripDir(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		if err := strip(filepath.Join(dir, e.Name())); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func strip(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := format.Node(&out, fset, f); err != nil {
		return err
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

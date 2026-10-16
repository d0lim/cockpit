package main

import (
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/d0lim/cockpit/internal/contract"
)

type compilerOptions struct {
	Target           string   `json:"target"`
	Module           string   `json:"module"`
	ModuleResolution string   `json:"moduleResolution"`
	Lib              []string `json:"lib"`
	Types            []string `json:"types"`
	Strict           bool     `json:"strict"`
	NoEmit           bool     `json:"noEmit"`
}

type tsconfig struct {
	CompilerOptions compilerOptions `json:"compilerOptions"`
	Files           []string        `json:"files"`
}

func main() {
	root := flag.String("root", ".", "repository root that holds schema/ and sdk/")
	flag.Parse()
	if err := generate(*root); err != nil {
		slog.Error("contract generation failed", "err", err)
		os.Exit(1)
	}
}

func generate(root string) error {
	schemas, err := buildSchemas()
	if err != nil {
		return err
	}
	for name, data := range schemas {
		if err := write(filepath.Join(root, "schema", name), data); err != nil {
			return err
		}
	}
	decls, err := dataDecls()
	if err != nil {
		return err
	}
	config := tsconfig{CompilerOptions: compilerOptions{
		Target:           "ES2022",
		Module:           "ESNext",
		ModuleResolution: "bundler",
		Lib:              []string{"ES2022"},
		Types:            []string{},
		Strict:           true,
		NoEmit:           true,
	}}
	if err := write(filepath.Join(root, "sdk", contract.GlobalsFile), []byte(contract.GlobalsTS)); err != nil {
		return err
	}
	config.Files = append(config.Files, "./"+contract.GlobalsFile)
	for _, m := range contract.Modules() {
		text, err := renderModule(m, decls)
		if err != nil {
			return err
		}
		if err := write(filepath.Join(root, "sdk", m.File), []byte(text)); err != nil {
			return err
		}
		config.Files = append(config.Files, "./"+m.File)
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return write(filepath.Join(root, "sdk", "tsconfig.base.json"), append(data, '\n'))
}

func write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

package seal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
)

const (
	fixtureRun    = "/fixture/runs/r-toy"
	fixtureRunID  = "r-toy"
	fixtureRunner = "cockpit-runner:0123456789ab"
	planted       = "PLANTED-secret-7f3a9c"
)

func identityPath(p string) (string, bool) {
	if !filepath.IsAbs(p) {
		return "", false
	}
	return filepath.Clean(p), true
}

func toyInput(t *testing.T) Input {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "config.toy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "toy-project", compose.DeclarationPath))
	if err != nil {
		t.Fatal(err)
	}
	decl, err := compose.ParseDeclaration(raw)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := compose.NewLayout(fixtureRun)
	if err != nil {
		t.Fatal(err)
	}
	files, err := compose.Files(layout, decl)
	if err != nil {
		t.Fatal(err)
	}
	return Input{
		Config:      config,
		Files:       files,
		Forbidden:   decl.Compose.Forbidden,
		RunID:       fixtureRunID,
		RunDir:      fixtureRun,
		Layout:      layout,
		PortBase:    22000,
		Ports:       decl.Ports,
		Builder:     decl.Project.Builder,
		RunnerImage: fixtureRunner,
		Realpath:    identityPath,
	}
}

func svc(t *testing.T, in Input, name string) map[string]any {
	t.Helper()
	s, ok := record(in.Config["services"])[name].(map[string]any)
	if !ok {
		t.Fatalf("fixture has no service %s", name)
	}
	return s
}

func edgePort(t *testing.T, in Input, i int) map[string]any {
	t.Helper()
	ports := list(svc(t, in, "edge")["ports"])
	if len(ports) <= i {
		t.Fatalf("edge has %d ports", len(ports))
	}
	return record(ports[i])
}

func setRoute(t *testing.T, in Input, i int, spec string) {
	t.Helper()
	command := list(svc(t, in, "edge")["command"])
	if len(command) <= i+1 {
		t.Fatalf("edge has %d routes", len(command)-1)
	}
	command[i+1] = spec
}

func failing(results []Result) []string {
	var out []string
	for _, r := range results {
		if !r.OK {
			out = append(out, r.Name)
		}
	}
	return out
}

package compose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationARecordingHarnessRendersThroughComposeWithTheRecorderAndItsWaits(t *testing.T) {
	ctx := integration(t)
	dir, sha := toyRepo(t, func(d string) {
		writeFile(t, filepath.Join(d, "extra.yaml"), "services:\n  worker:\n    image: docker.io/library/alpine:3.22\n    depends_on: [cache]\n")
		replaceIn(t, filepath.Join(d, DeclarationPath), "  files: [compose.yaml, compose.sealed-app.yaml]\n", "  files: [compose.yaml, compose.sealed-app.yaml, extra.yaml]\n")
		replaceIn(t, filepath.Join(d, DeclarationPath), "telemetry:\n", "wire:\n  routes:\n    - { name: app, service: app, port: 8080 }\n  publicFields:\n    - { key: token, value: \"tok-[0-9a-f]{8}$\" }\ntelemetry:\n")
	})
	loaded, err := ReadDeclaration(ctx, Git{Dir: dir}, sha, "")
	if err != nil {
		t.Fatal(err)
	}
	l := testLayout(t)
	root, err := OpenRun(l.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := Extract(ctx, loaded.Tree, root); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{".env": "POSTGRES_USER=toy\nPOSTGRES_DB=toy\nPOSTGRES_PASSWORD=fixture-password-0123456789\nAPP_TOKEN=toy_fixturefixturefixturefi\n", PrivateEnvName: `{"local":{}}`} {
		if err := os.WriteFile(filepath.Join(l.Src, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	decl := loaded.Declaration
	if err := MakeWorkTree(root, decl, ""); err != nil {
		t.Fatal(err)
	}
	keys, err := ImageKeys(loaded)
	if err != nil {
		t.Fatal(err)
	}
	const base = 22500
	routes := WireRoutes(decl)
	if err := WriteRunEnv(root, RunEnvInput{RunID: runID, Layout: l, PortBase: base, Ports: decl.Ports, Keys: keys, Wire: routes}); err != nil {
		t.Fatal(err)
	}
	if err := WriteHarness(root, HarnessInput{RunID: runID, RunnerImage: "cockpit-runner:0123456789ab", PortBase: base, Layout: l, Loaded: loaded, Wire: true}); err != nil {
		t.Fatal(err)
	}
	args, err := ComposeArgs(runID, l, decl)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Render(ctx, CLI{}, args, func(s string) string { return s })
	if err != nil {
		t.Fatal(err)
	}
	if err := AssertRoles(r, runID, decl.Project.Builder, OTLPDeclared(decl), true); err != nil {
		t.Fatal(err)
	}
	res, err := CLI{}.Run(ctx, Command{Args: append(append([]string{}, args...), "--profile", "*", "config", "--format", "json")})
	if err != nil || res.Code != 0 {
		t.Fatalf("compose config: exit %d %v\n%s", res.Code, err, res.Stderr)
	}
	var config struct {
		Services map[string]struct {
			DependsOn map[string]struct {
				Condition string `json:"condition"`
			} `json:"depends_on"`
			Environment map[string]string `json:"environment"`
			Command     []string          `json:"command"`
		} `json:"services"`
	}
	if err := json.Unmarshal(res.Stdout, &config); err != nil {
		t.Fatal(err)
	}
	for _, name := range loaded.Model.ServiceNames() {
		got := config.Services[name].DependsOn[RoleWire].Condition
		if name == decl.Project.Builder {
			if got != "" {
				t.Errorf("the builder waits for the recorder: %q", got)
			}
			continue
		}
		if got != "service_healthy" {
			t.Errorf("%s waits with %q", name, got)
		}
	}
	if config.Services["worker"].DependsOn["cache"].Condition == "" {
		t.Errorf("the short depends_on of worker was lost: %v", config.Services["worker"].DependsOn)
	}
	if config.Services["app"].DependsOn["seed"].Condition != "service_completed_successfully" {
		t.Errorf("the depends_on of app was replaced: %v", config.Services["app"].DependsOn)
	}
	if got := config.Services[RoleRunner].Environment["COCKPIT_WIRE_ROUTES"]; got != "app=app:8080=cockpit-wire:41001" {
		t.Errorf("runner routes %q", got)
	}
	if got := strings.Join(config.Services[RoleWire].Command, " "); !strings.HasPrefix(got, "wire --project "+runID+" --public-field token=") || !strings.HasSuffix(got, " 41001=app:8080") {
		t.Errorf("recorder command %q", got)
	}
	if env := readFile(t, l.RunEnv()); !strings.Contains(env, "COCKPIT_WIRE_APP=cockpit-wire:41001\n") {
		t.Errorf("run.env %s", env)
	}
}

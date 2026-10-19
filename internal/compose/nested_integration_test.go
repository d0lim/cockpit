package compose

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

const nestedBase = 22500

type nestedService struct {
	Build struct {
		Context string `json:"context"`
	} `json:"build"`
	Volumes []struct {
		Type   string `json:"type"`
		Source string `json:"source"`
		Target string `json:"target"`
	} `json:"volumes"`
}

func nestToy(t *testing.T, dir string) {
	t.Helper()
	stack := filepath.Join(dir, "stack")
	if err := os.MkdirAll(filepath.Join(stack, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(stack, "config", "app.conf"), "mode=toy\n")
	for _, name := range []string{"compose.yaml", "compose.sealed-app.yaml"} {
		if err := os.Rename(filepath.Join(dir, name), filepath.Join(stack, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Rename(filepath.Join(dir, "mock"), filepath.Join(stack, "server")); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(stack, "compose.yaml")
	replaceIn(t, base, "context: ./mock", "context: ./server")
	replaceIn(t, base, "./db/init.sql:", "../db/init.sql:")
	replaceIn(t, base, "    ports:\n      - \"8080:8080\"", "    volumes:\n      - ./config:/etc/app:ro\n    ports:\n      - \"8080:8080\"")
	sealed := filepath.Join(stack, "compose.sealed-app.yaml")
	for range 2 {
		replaceIn(t, sealed, "${RUN_BUILD:?}/mock", "${RUN_BUILD:?}/stack/server")
	}
	replaceIn(t, filepath.Join(dir, "make-env.sh"), "cat > .env", "cat > stack/.env")
	decl := filepath.Join(dir, DeclarationPath)
	replaceIn(t, decl, "dirty: [.cockpit, compose.yaml, compose.sealed-app.yaml, db, fixtures, mock,", "dirty: [.cockpit, stack, db, fixtures,")
	replaceIn(t, decl, "files: [compose.yaml, compose.sealed-app.yaml]", "files: [stack/compose.yaml, stack/compose.sealed-app.yaml]\n  projectDirectory: stack")
	replaceIn(t, decl, "inputs: [mock]", "inputs: [stack/server]")
	replaceIn(t, decl, "outputs: [.env,", "outputs: [stack/.env,")
	replaceIn(t, decl, "path: .env, format", "path: stack/.env, format")
}

func renderServices(ctx context.Context, t *testing.T, root *os.Root, l Layout, loaded Loaded) map[string]nestedService {
	t.Helper()
	if err := WriteHarness(root, HarnessInput{RunID: runID, RunnerImage: "cockpit-runner:0123456789ab", PortBase: nestedBase, Layout: l, Loaded: loaded}); err != nil {
		t.Fatal(err)
	}
	args, err := ComposeArgs(runID, l, loaded.Declaration)
	if err != nil {
		t.Fatal(err)
	}
	res, err := CLI{}.Run(ctx, Command{Args: append(args, "--profile", "*", "config", "--format", "json")})
	if err != nil || res.Code != 0 {
		t.Fatalf("compose config: exit %d %v\n%s", res.Code, err, hubMasker(t)(string(res.Stderr)))
	}
	var config struct {
		Services map[string]nestedService `json:"services"`
	}
	if err := json.Unmarshal(res.Stdout, &config); err != nil {
		t.Fatal(err)
	}
	return config.Services
}

func bindSource(t *testing.T, svc nestedService, target string) string {
	t.Helper()
	for _, v := range svc.Volumes {
		if v.Type == "bind" && v.Target == target {
			return v.Source
		}
	}
	t.Fatalf("no bind on %s in %+v", target, svc.Volumes)
	return ""
}

func prepareNested(ctx context.Context, t *testing.T) (*os.Root, Layout, Loaded) {
	t.Helper()
	dir, sha := toyRepo(t, func(dir string) { nestToy(t, dir) })
	g := Git{Dir: dir}
	loaded, err := ReadDeclaration(ctx, g, sha)
	if err != nil {
		t.Fatal(err)
	}
	l := testLayout(t)
	root, err := OpenRun(l.Root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	if err := Extract(ctx, g, loaded.Tree, root); err != nil {
		t.Fatal(err)
	}
	decl := loaded.Declaration
	if err := GenerateEnv(ctx, CLI{}, root, EnvInput{RunID: runID, Layout: l, Env: decl.Env, User: EnvUser(os.Getuid(), os.Getgid()), Mask: hubMasker(t)}); err != nil {
		t.Fatal(err)
	}
	if err := MakeWorkTree(root, decl); err != nil {
		t.Fatal(err)
	}
	keys, err := ImageKeys(decl, loaded.Tree)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteRunEnv(root, RunEnvInput{RunID: runID, Layout: l, PortBase: nestedBase, Ports: decl.Ports, Keys: keys}); err != nil {
		t.Fatal(err)
	}
	return root, l, loaded
}

func TestIntegrationComposeFilesInASubdirectoryResolveBuildContextsAndBindsUnderTheProjectDirectory(t *testing.T) {
	ctx := integration(t)
	root, l, loaded := prepareNested(ctx, t)
	sealed := renderServices(ctx, t, root, l, loaded)
	if got := bindSource(t, sealed["app"], "/etc/app"); got != filepath.Join(l.Src, "stack", "config") {
		t.Errorf("app ./config resolved to %s", got)
	}
	if got := bindSource(t, sealed["db"], "/docker-entrypoint-initdb.d/init.sql"); got != filepath.Join(l.Src, "db", "init.sql") {
		t.Errorf("db ../db/init.sql resolved to %s", got)
	}
	if got := sealed["app"].Build.Context; got != filepath.Join(l.Build, "stack", "server") {
		t.Errorf("sealed app context %s", got)
	}
	plain := loaded
	plain.Declaration.Compose.Files = []contract.RelPath{"stack/compose.yaml"}
	plain.Declaration.Project.Builder = ""
	model, err := ParseModel([]byte(readFile(t, filepath.Join(l.Src, "stack", "compose.yaml"))))
	if err != nil {
		t.Fatal(err)
	}
	plain.Model = model
	if got := renderServices(ctx, t, root, l, plain)["app"].Build.Context; got != filepath.Join(l.Src, "stack", "server") {
		t.Errorf("app ./server resolved to %s", got)
	}
	rooted := plain
	rooted.Declaration.Compose.ProjectDirectory = ""
	rooted.Declaration.Env.Outputs = []contract.RelPath{".env"}
	writeFile(t, filepath.Join(l.Src, ".env"), readFile(t, filepath.Join(l.Src, "stack", ".env")))
	canary := renderServices(ctx, t, root, l, rooted)
	if got := canary["app"].Build.Context; got != filepath.Join(l.Src, "server") {
		t.Fatalf("without a project directory app ./server resolved to %s, not against the commit root", got)
	}
	if got := bindSource(t, canary["app"], "/etc/app"); got != filepath.Join(l.Src, "config") {
		t.Fatalf("without a project directory app ./config resolved to %s, not against the commit root", got)
	}
}

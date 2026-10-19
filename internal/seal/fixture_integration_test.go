package seal

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
)

const (
	updateFixture  = "COCKPIT_UPDATE_SEAL_FIXTURE"
	fixturePath    = "testdata/config.toy.json"
	fixturePorts   = 22000
	fixtureDotenv  = "POSTGRES_USER=toy\nPOSTGRES_DB=toy\nPOSTGRES_PASSWORD=fixture-password-0123456789\nAPP_TOKEN=toy_fixturefixturefixturefi\n"
	fixtureTimeout = 3 * time.Minute
)

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=toy", "GIT_AUTHOR_EMAIL=toy@example.invalid", "GIT_COMMITTER_NAME=toy", "GIT_COMMITTER_EMAIL=toy@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func toyCommit(t *testing.T) (compose.Git, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "toy")
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "testdata", "toy-project"))); err != nil {
		t.Fatal(err)
	}
	for _, generated := range []string{".env", compose.PrivateEnvName, "mock/bin"} {
		os.RemoveAll(filepath.Join(dir, generated))
	}
	fixtureGit(t, dir, "init", "-q", "-b", "main")
	fixtureGit(t, dir, "add", "-A")
	fixtureGit(t, dir, "commit", "-q", "-m", "toy")
	return compose.Git{Dir: dir}, fixtureGit(t, dir, "rev-parse", "HEAD")
}

func renderToy(ctx context.Context, t *testing.T) map[string]any {
	t.Helper()
	g, sha := toyCommit(t)
	loaded, err := compose.ReadDeclaration(ctx, g, sha)
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(t.TempDir(), fixtureRunID)
	l, err := compose.NewLayout(runDir)
	if err != nil {
		t.Fatal(err)
	}
	root, err := compose.OpenRun(l.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := compose.Extract(ctx, g, loaded.Tree, root); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{".env": fixtureDotenv, compose.PrivateEnvName: `{"local":{}}`} {
		if err := os.WriteFile(filepath.Join(l.Src, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	decl := loaded.Declaration
	if err := compose.MakeWorkTree(root, decl); err != nil {
		t.Fatal(err)
	}
	keys, err := compose.ImageKeys(decl, loaded.Tree)
	if err != nil {
		t.Fatal(err)
	}
	if err := compose.WriteRunEnv(root, compose.RunEnvInput{RunID: fixtureRunID, Layout: l, PortBase: fixturePorts, Ports: decl.Ports, Keys: keys}); err != nil {
		t.Fatal(err)
	}
	if err := compose.WriteHarness(root, compose.HarnessInput{RunID: fixtureRunID, RunnerImage: fixtureRunner, PortBase: fixturePorts, Layout: l, Loaded: loaded}); err != nil {
		t.Fatal(err)
	}
	args, err := compose.ComposeArgs(fixtureRunID, l, decl)
	if err != nil {
		t.Fatal(err)
	}
	res, err := compose.CLI{}.Run(ctx, compose.Command{Args: append(args, "--profile", "*", "config", "--format", "json")})
	if err != nil || res.Code != 0 {
		t.Fatalf("compose config: exit %d %v\n%s", res.Code, err, res.Stderr)
	}
	text := string(res.Stdout)
	if real, err := filepath.EvalSymlinks(l.Root); err == nil {
		text = strings.ReplaceAll(text, real, fixtureRun)
	}
	text = strings.ReplaceAll(text, l.Root, fixtureRun)
	var config map[string]any
	if err := json.Unmarshal([]byte(text), &config); err != nil {
		t.Fatal(err)
	}
	return normalizeFixture(config)
}

func normalizeFixture(config map[string]any) map[string]any {
	for _, raw := range record(config["services"]) {
		svc := record(raw)
		if _, ok := svc["environment"]; ok {
			svc["environment"] = map[string]any{}
		}
	}
	return config
}

func TestIntegrationTheCommittedSealFixtureEqualsAFreshRenderOfTheToyConsumer(t *testing.T) {
	if os.Getenv("COCKPIT_INTEGRATION") != "1" {
		t.Skip("needs COCKPIT_INTEGRATION=1 and docker compose")
	}
	ctx, cancel := context.WithTimeout(context.Background(), fixtureTimeout)
	defer cancel()
	fresh := renderToy(ctx, t)
	if os.Getenv(updateFixture) == "1" {
		data, err := json.MarshalIndent(fresh, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixturePath, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s", fixturePath)
	}
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var committed map[string]any
	if err := json.Unmarshal(data, &committed); err != nil {
		t.Fatal(err)
	}
	services := record(fresh["services"])
	if len(services) != 8 || strings.Contains(string(data), "fixture-password") {
		t.Fatalf("fresh render has %d services or the fixture holds a credential", len(services))
	}
	if !reflect.DeepEqual(committed, fresh) {
		want, _ := json.MarshalIndent(fresh, "", "  ")
		t.Fatalf("%s differs from a fresh render; run make seal-fixture and review the diff\nfresh render:\n%s", fixturePath, want)
	}
}

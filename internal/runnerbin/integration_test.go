package runnerbin

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type countingDocker struct {
	CLI
	imports int
}

func (c *countingDocker) Run(ctx context.Context, stdin io.Reader, args ...string) (string, error) {
	if args[0] == "import" {
		c.imports++
	}
	return c.CLI.Run(ctx, stdin, args...)
}

const integrationSuite = `import { type Ctx, flow, stage, step, suite } from "cockpit:flow";

export default suite({
	name: "fake",
	stages: [stage({ id: "boot", title: "boot" })],
	flows: [
		flow({
			id: "alpha",
			title: "alpha",
			stage: "boot",
			once: false,
			requires: [],
			steps: [
				step({
					id: "seed",
					title: "seed",
					kind: "capture",
					run: async (ctx: Ctx) => {
						ctx.saveSeed("admin-1", "SEEDTWOAAAAABBBBBCCCCCDDDDDEEEE");
						ctx.capture("note", "kept", { persist: true });
					},
				}),
			],
		}),
	],
});
`

func dockerOut(t *testing.T, ctx context.Context, args ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func TestIntegrationEmbeddedRunnerImageReportsSameVersionHasNoShellAndWritesBindsAsNonRoot(t *testing.T) {
	if os.Getenv("COCKPIT_DOCKER") != "1" {
		t.Skip("needs COCKPIT_DOCKER=1 and scripts/build-runners.sh")
	}
	version, err := os.ReadFile("bin/VERSION")
	if err != nil {
		t.Skip("bin/VERSION is missing; run scripts/build-runners.sh first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	d := &countingDocker{}
	pins := FilePins{Path: filepath.Join(t.TempDir(), "runner-images.json")}
	img, err := Ensure(ctx, d, Lookup, pins)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Ensure(ctx, d, Lookup, pins)
	if err != nil || again != img || d.imports != 1 {
		t.Fatalf("second ensure: %+v %v imports=%d", again, err, d.imports)
	}
	platform := "linux/" + img.Arch
	out, err := dockerOut(t, ctx, "run", "--rm", "--platform", platform, img.Tag, "--version")
	if err != nil || out != "cockpit-runner "+strings.TrimSpace(string(version)) {
		t.Fatalf("--version: %q %v", out, err)
	}
	if out, err := dockerOut(t, ctx, "run", "--rm", "--platform", platform, "--entrypoint", "sh", img.Tag, "-c", "true"); err == nil || !strings.Contains(out, "executable file not found") {
		t.Fatalf("sh is present: %q %v", out, err)
	}
	root := t.TempDir()
	scripts := filepath.Join(root, "scripts")
	e2e := filepath.Join(root, "e2e")
	privDir := filepath.Join(root, "priv")
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(scripts, "suites"), 0o755))
	must(os.MkdirAll(e2e, 0o700))
	must(os.MkdirAll(privDir, 0o700))
	must(os.WriteFile(filepath.Join(scripts, "suites", "fake.ts"), []byte(integrationSuite), 0o644))
	must(os.WriteFile(filepath.Join(scripts, "env.json"), []byte(`{"totpSeedKeys":{"admin-1":"totpSecret1"}}`), 0o644))
	must(os.WriteFile(filepath.Join(root, "project.yaml"), []byte("hostApi: 1\n"), 0o644))
	priv := filepath.Join(privDir, "e2e.private.env.json")
	must(os.WriteFile(priv, []byte(`{"local":{"credential":"PLANTED-cred-7f3a9c"}}`), 0o600))
	out, err = dockerOut(t, ctx, "run", "--rm", "--platform", platform,
		"-v", scripts+":/work/scripts:ro",
		"-v", filepath.Join(root, "project.yaml")+":/work/.cockpit/project.yaml:ro",
		"-v", e2e+":/work/e2e",
		"-v", priv+":/work/e2e/e2e.private.env.json",
		"-e", "COCKPIT_RUNNER_IMAGE="+img.Tag,
		img.Tag, "run", "fake")
	if err != nil || !strings.Contains(out, "alpha › seed passed") {
		t.Fatalf("run fake: %q %v", out, err)
	}
	for _, name := range []string{"manifest.json", "result.json", "ledger.json", "context.json"} {
		if _, err := os.Stat(filepath.Join(e2e, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	var manifest struct {
		Runner struct {
			Version string `json:"version"`
			Image   string `json:"image"`
		} `json:"runner"`
	}
	raw, _ := os.ReadFile(filepath.Join(e2e, "manifest.json"))
	if err := json.Unmarshal(raw, &manifest); err != nil || manifest.Runner.Image != img.Tag || manifest.Runner.Version != strings.TrimSpace(string(version)) {
		t.Fatalf("manifest runner %+v %v", manifest.Runner, err)
	}
	var doc map[string]map[string]string
	raw, _ = os.ReadFile(priv)
	info, _ := os.Stat(priv)
	if err := json.Unmarshal(raw, &doc); err != nil || doc["local"]["totpSecret1"] != "SEEDTWOAAAAABBBBBCCCCCDDDDDEEEE" || doc["local"]["credential"] == "" || info.Mode().Perm() != 0o600 {
		t.Fatalf("private env %s mode %v err %v", raw, info.Mode(), err)
	}
}

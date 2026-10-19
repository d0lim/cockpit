package compose

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
)

func integration(t *testing.T) context.Context {
	t.Helper()
	if os.Getenv("COCKPIT_INTEGRATION") != "1" {
		t.Skip("needs COCKPIT_INTEGRATION=1 and a Docker daemon")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func hubMasker(t *testing.T) func(string) string {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Hub})
	if err != nil {
		t.Fatal(err)
	}
	return m.Mask
}

func TestIntegrationEnvContainerCannotReachTheNetworkSeesNoHostVariablesAndWritesAt600(t *testing.T) {
	ctx := integration(t)
	t.Setenv("COCKPIT_PLANTED_HOST", planted)
	l := testLayout(t)
	root := runRoot(t, l)
	script := `env > env.txt
{ id -u; umask; command -v wget >/dev/null && echo wget=present
if wget -q -T 3 -O /dev/null http://1.1.1.1/ 2>/dev/null; then echo net=reached; else echo net=blocked; fi
stat -c %a env.txt; } > probe.txt`
	in := EnvInput{RunID: runID, Layout: l, User: EnvUser(os.Getuid(), os.Getgid()), Mask: hubMasker(t), Env: contract.EnvSection{
		Command: []string{"sh", "-c", script},
		Image:   toyDeclaration(t).Env.Image,
		Outputs: []contract.RelPath{"env.txt", "probe.txt"},
	}}
	if err := GenerateEnv(ctx, CLI{}, root, in); err != nil {
		t.Fatal(err)
	}
	env := readFile(t, filepath.Join(l.Src, "env.txt"))
	probe := strings.Fields(readFile(t, filepath.Join(l.Src, "probe.txt")))
	if strings.Contains(env, planted) || strings.Contains(env, "COCKPIT_PLANTED_HOST") || !strings.Contains(env, "COMPOSE_PROJECT_NAME="+runID) {
		t.Fatalf("container environment:\n%s", env)
	}
	if len(probe) != 5 || probe[0] == "0" || probe[1] != "0077" || probe[2] != "wget=present" || probe[3] != "net=blocked" || probe[4] != "600" {
		t.Fatalf("probe output %q", probe)
	}
	for _, name := range []string{"env.txt", "probe.txt"} {
		if mode(t, filepath.Join(l.Src, name)) != 0o600 {
			t.Errorf("%s mode %v", name, mode(t, filepath.Join(l.Src, name)))
		}
	}
}

func TestIntegrationToyHarnessRendersWithEveryRoleRunLabelsAndPortsOnlyOnTheEdge(t *testing.T) {
	ctx := integration(t)
	dir, sha := toyRepo(t, nil)
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
	defer root.Close()
	if err := Extract(ctx, g, loaded.Tree, root); err != nil {
		t.Fatal(err)
	}
	decl := loaded.Declaration
	envIn := EnvInput{RunID: runID, Layout: l, Env: decl.Env, User: EnvUser(os.Getuid(), os.Getgid()), Mask: hubMasker(t)}
	if err := GenerateEnv(ctx, CLI{}, root, envIn); err != nil {
		t.Fatal(err)
	}
	if err := MakeWorkTree(root, decl); err != nil {
		t.Fatal(err)
	}
	keys, err := ImageKeys(decl, loaded.Tree)
	if err != nil {
		t.Fatal(err)
	}
	const base = 22400
	if err := WriteRunEnv(root, RunEnvInput{RunID: runID, Layout: l, PortBase: base, Ports: decl.Ports, Keys: keys}); err != nil {
		t.Fatal(err)
	}
	if err := WriteHarness(root, HarnessInput{RunID: runID, RunnerImage: "cockpit-runner:0123456789ab", PortBase: base, Layout: l, Loaded: loaded}); err != nil {
		t.Fatal(err)
	}
	args, err := ComposeArgs(runID, l, decl)
	if err != nil {
		t.Fatal(err)
	}
	values, err := mask.Refresh(MaskSources(decl, l))
	if err != nil {
		t.Fatal(err)
	}
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: values.Known, Patterns: decl.Secrets.Patterns})
	if err != nil {
		t.Fatal(err)
	}
	r, err := Render(ctx, CLI{}, args, m.Mask)
	if err != nil {
		t.Fatal(err)
	}
	if err := AssertRoles(r, runID, decl.Project.Builder); err != nil {
		t.Fatal(err)
	}
	if want := len(loaded.Model.Services) + len(HarnessServices); want != 9 || len(r.Services) != want || r.Services["app"].Image != "toy/app:"+keys["IMG_APP"] {
		t.Fatalf("services %d, app image %q", len(r.Services), r.Services["app"].Image)
	}
	if !r.Networks[DefaultNetwork].Internal || r.Networks[EgressNetwork].Internal {
		t.Fatalf("networks %+v", r.Networks)
	}
	published := 0
	for name, svc := range r.Services {
		if len(svc.Ports) > 0 && name != RoleEdge {
			t.Errorf("%s publishes %d ports", name, len(svc.Ports))
		}
		published += len(svc.Ports)
	}
	if published != len(decl.Ports) {
		t.Fatalf("%d ports published, %d declared", published, len(decl.Ports))
	}
}

package compose

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var envgenMutations = []porttest.Mutation{
	{"the env container has no network", "drop --network none from EnvArgs", "red: the argument check fails and the integration run reaches 1.1.1.1", "TestEnvContainerHasNoNetworkRunsAsNonRootBindsOnlySrcAndReceivesOnlyTheProjectName"},
	{"the env container sees only the project name", "add --env HOME or pass the host environment", "red: a second --env appears", "TestEnvContainerHasNoNetworkRunsAsNonRootBindsOnlySrcAndReceivesOnlyTheProjectName"},
	{"the env container never runs as root", "let EnvUser return 0:0 for the root hub", "red: the hub running as root yields user 0", "TestEnvUserFallsBackToANonRootUserWhenTheHubRunsAsRoot"},
	{"outputs are 600", "drop the Chmod in sealOutputs", "red: an output written 644 stays 644", "TestGeneratedOutputsEndAt600AndAMissingOrLinkedOutputFailsGeneration"},
	{"a failed generation removes only its own outputs and masks stderr", "drop the cleanup loop or the in.Mask call", "red: a produced output survives, a committed one is removed, or the planted value shows", "TestFailedEnvCommandRemovesOnlyItsOwnOutputsAndMasksTheStderrTail"},
	{"every env output is a masker value source", "build MaskSources from secrets.sources only", "red: the planted value in e2e.private.env.json is not masked", "TestValuePlantedInAnOutputOutsideSecretsSourcesIsMaskedAfterRefresh"},
}

func TestEnvgenMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "envgen_test.go", envgenMutations)
}

func envInput(t *testing.T, l Layout) EnvInput {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	return EnvInput{RunID: runID, Layout: l, Env: toyDeclaration(t).Env, User: EnvUser(501, 20), Mask: m.Mask}
}

func valuesAfter(args []string, flag string) []string {
	var out []string
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			out = append(out, args[i+1])
		}
	}
	return out
}

func TestEnvContainerHasNoNetworkRunsAsNonRootBindsOnlySrcAndReceivesOnlyTheProjectName(t *testing.T) {
	l := testLayout(t)
	in := envInput(t, l)
	args := EnvArgs(in)
	checks := map[string][]string{
		"--network":  {"none"},
		"--user":     {"501:20"},
		"--mount":    {"type=bind,source=" + l.Src + ",target=/src"},
		"--env":      {"COMPOSE_PROJECT_NAME=" + runID},
		"--cap-drop": {"ALL"},
	}
	for flag, want := range checks {
		if got := valuesAfter(args, flag); !slices.Equal(got, want) {
			t.Errorf("%s: %q, want %q", flag, got, want)
		}
	}
	for _, banned := range []string{"-v", "--volume", "-e", "--env-file", "--privileged", "--network=host"} {
		if slices.Contains(args, banned) {
			t.Errorf("argument %s is present", banned)
		}
	}
	if args[0] != "run" || !slices.Contains(args, "--rm") || !slices.Contains(args, "--read-only") {
		t.Errorf("args %q", args)
	}
	image := string(toyDeclaration(t).Env.Image)
	at := slices.Index(args, image)
	if at < 0 || !strings.Contains(image, "@sha256:") || !slices.Equal(args[at+1:], []string{"-c", `umask 077 && exec "$@"`, "cockpit-env", "sh", "make-env.sh"}) {
		t.Fatalf("command after the image: %q", args[max(at, 0):])
	}
}

func TestEnvUserFallsBackToANonRootUserWhenTheHubRunsAsRoot(t *testing.T) {
	cases := map[[2]int]string{{0, 0}: "65532:65532", {501, 20}: "501:20", {1000, 1000}: "1000:1000", {-1, -1}: "65532:65532"}
	for ids, want := range cases {
		if got := EnvUser(ids[0], ids[1]); got != want {
			t.Errorf("EnvUser(%d, %d) = %s, want %s", ids[0], ids[1], got, want)
		}
	}
}

func runRoot(t *testing.T, l Layout) *os.Root {
	t.Helper()
	root, err := OpenRun(l.Root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	if err := os.MkdirAll(l.Src, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func writing(l Layout, files map[string]string, code int, stderr string) *fakeDocker {
	return &fakeDocker{run: func(Command) (Result, error) {
		for name, data := range files {
			os.WriteFile(filepath.Join(l.Src, name), []byte(data), 0o644)
		}
		return Result{Code: code, Stderr: []byte(stderr)}, nil
	}}
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestGeneratedOutputsEndAt600AndAMissingOrLinkedOutputFailsGeneration(t *testing.T) {
	l := testLayout(t)
	root := runRoot(t, l)
	d := writing(l, map[string]string{".env": "A=1\n", PrivateEnvName: "{}"}, 0, "")
	if err := GenerateEnv(context.Background(), d, root, envInput(t, l)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".env", PrivateEnvName} {
		if got := mode(t, filepath.Join(l.Src, name)); got != 0o600 {
			t.Errorf("%s mode %v", name, got)
		}
	}
	if len(d.calls) != 1 {
		t.Fatalf("%d docker calls", len(d.calls))
	}
	l2 := testLayout(t)
	root2 := runRoot(t, l2)
	err := GenerateEnv(context.Background(), writing(l2, map[string]string{".env": "A=1\n"}, 0, ""), root2, envInput(t, l2))
	if err == nil || !strings.Contains(err.Error(), PrivateEnvName) || exists(filepath.Join(l2.Src, ".env")) {
		t.Fatalf("missing output: %v", err)
	}
	l3 := testLayout(t)
	root3 := runRoot(t, l3)
	linked := &fakeDocker{run: func(Command) (Result, error) {
		os.WriteFile(filepath.Join(l3.Src, ".env"), []byte("A=1\n"), 0o644)
		os.Symlink("/etc/hosts", filepath.Join(l3.Src, PrivateEnvName))
		return Result{}, nil
	}}
	if err := GenerateEnv(context.Background(), linked, root3, envInput(t, l3)); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("linked output: %v", err)
	}
}

func TestFailedEnvCommandRemovesOnlyItsOwnOutputsAndMasksTheStderrTail(t *testing.T) {
	l := testLayout(t)
	root := runRoot(t, l)
	writeFile(t, filepath.Join(l.Src, ".env"), "COMMITTED=1\n")
	d := writing(l, map[string]string{PrivateEnvName: "{}"}, 3, "line one\nline two\ntoken="+planted+"\n")
	err := GenerateEnv(context.Background(), d, root, envInput(t, l))
	if err == nil {
		t.Fatal("a failing env command passed")
	}
	msg := err.Error()
	if strings.Contains(msg, planted) || !strings.Contains(msg, "****") || !strings.Contains(msg, "exit code 3") || !strings.Contains(msg, "line two") {
		t.Fatalf("message %q", msg)
	}
	if exists(filepath.Join(l.Src, PrivateEnvName)) {
		t.Fatal("the produced output survived the failure")
	}
	if readFile(t, filepath.Join(l.Src, ".env")) != "COMMITTED=1\n" {
		t.Fatal("an output that existed before the run was removed")
	}
}

func TestValuePlantedInAnOutputOutsideSecretsSourcesIsMaskedAfterRefresh(t *testing.T) {
	l := testLayout(t)
	decl := toyDeclaration(t)
	if slices.ContainsFunc(decl.Secrets.Sources, func(s contract.SecretSource) bool { return strings.HasSuffix(string(s.Path), PrivateEnvName) }) {
		t.Fatal("the private env must stay outside secrets.sources for this test")
	}
	writeFile(t, filepath.Join(l.Src, ".env"), "POSTGRES_USER=toyuser1\nPOSTGRES_PASSWORD=dbpass-0123456789\n")
	writeFile(t, filepath.Join(l.Src, PrivateEnvName), `{"local":{"appToken":"`+planted+`"}}`)
	src := MaskSources(decl, l)
	if len(src.Outputs) != 2 || src.Outputs[1].Kind != mask.JSON || src.Outputs[0].Kind != mask.Dotenv {
		t.Fatalf("sources %+v", src)
	}
	values, err := mask.Refresh(src)
	if err != nil {
		t.Fatal(err)
	}
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: values.Known, Patterns: decl.Secrets.Patterns})
	if err != nil {
		t.Fatal(err)
	}
	out := m.Mask("request carried " + planted + " for toyuser1")
	if strings.Contains(out, planted) || !strings.Contains(out, "****") {
		t.Fatalf("masked %q", out)
	}
	if !strings.Contains(out, "toyuser1") {
		t.Fatalf("a public name value was masked: %q", out)
	}
}

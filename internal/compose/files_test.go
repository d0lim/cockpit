package compose

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var filesMutations = []porttest.Mutation{
	{"compose files matching compose.forbidden cannot join a sealed run", "make AssertSealedFiles return nil", "red: tunnel and outbound overlays pass", "TestForbiddenComposeFileNamesAreRejectedInAnyDirectoryOrCaseAndOtherFilesPass"},
	{"every compose call names the project, its directory and the generated harness", "leave the harness out of Files", "red: the argument list lacks -f harness.yaml", "TestComposeArgsNameProjectDirectoryEnvFilesAndEveryFileWithTheHarnessLast"},
	{"compose resolves relative paths against the declared project directory and reads its .env", "pass l.Src as --project-directory and keep the root .env", "red: the project directory is the commit root and the env file is the root .env", "TestDeclaredProjectDirectoryBecomesTheComposeDirectoryAndItsDotenvTheEnvFile"},
	{"run directories are absolute", "drop the IsAbs check in NewLayout", "red: a relative run directory is accepted", "TestRelativeRunDirectoryIsRejected"},
}

func TestFilesMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "files_test.go", filesMutations)
}

func TestForbiddenComposeFileNamesAreRejectedInAnyDirectoryOrCaseAndOtherFilesPass(t *testing.T) {
	forbidden := []string{"tunnel", "outbound"}
	denied := []string{"/run/src/compose.tunnel.yaml", "/run/src/deep/dir/Relay-TUNNEL.yml", "/run/src/copy-of-outbound.yaml"}
	allowed := []string{"/run/src/compose.yaml", "/run/src/compose.sealed-app.yaml", "/run/tunnels-dir/compose.yaml"}
	for _, f := range denied {
		err := AssertSealedFiles(append(slices.Clone(allowed), f), forbidden)
		if err == nil || !strings.Contains(err.Error(), f) {
			t.Errorf("%s: %v", f, err)
		}
	}
	if err := AssertSealedFiles(allowed, forbidden); err != nil {
		t.Fatalf("allowed files were rejected: %v", err)
	}
	if err := AssertSealedFiles(denied, nil); err != nil {
		t.Fatalf("files were rejected without a forbidden list: %v", err)
	}
	decl := toyDeclaration(t)
	decl.Compose.Files = append(decl.Compose.Files, "compose.tunnel.yaml")
	if _, err := ComposeArgs(runID, testLayout(t), decl); err == nil {
		t.Fatal("compose args were built with a forbidden file")
	}
}

func TestComposeArgsNameProjectDirectoryEnvFilesAndEveryFileWithTheHarnessLast(t *testing.T) {
	l := testLayout(t)
	args, err := ComposeArgs(runID, l, toyDeclaration(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"compose", "--ansi", "never", "--progress", "plain", "-p", runID, "--project-directory", l.Src,
		"--env-file", filepath.Join(l.Src, ".env"), "--env-file", filepath.Join(l.Root, "env", "run.env"),
		"-f", filepath.Join(l.Src, "compose.yaml"), "-f", filepath.Join(l.Src, "compose.sealed-app.yaml"),
		"-f", filepath.Join(l.Root, "harness.yaml"),
	}
	if !slices.Equal(args, want) {
		t.Fatalf("args\n%q\nwant\n%q", args, want)
	}
	decl := toyDeclaration(t)
	decl.Env.Outputs = nil
	args, err = ComposeArgs(runID, l, decl)
	if err != nil || slices.Contains(args, filepath.Join(l.Src, ".env")) {
		t.Fatalf("without a root .env output: %q %v", args, err)
	}
}

func TestDeclaredProjectDirectoryBecomesTheComposeDirectoryAndItsDotenvTheEnvFile(t *testing.T) {
	l := testLayout(t)
	decl := toyDeclaration(t)
	decl.Compose.ProjectDirectory = "backend/app"
	decl.Env.Outputs = []contract.RelPath{".env", "backend/app/.env"}
	args, err := ComposeArgs(runID, l, decl)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"compose", "--ansi", "never", "--progress", "plain", "-p", runID, "--project-directory", filepath.Join(l.Src, "backend", "app"),
		"--env-file", filepath.Join(l.Src, "backend", "app", ".env"), "--env-file", filepath.Join(l.Root, "env", "run.env"),
		"-f", filepath.Join(l.Src, "compose.yaml"), "-f", filepath.Join(l.Src, "compose.sealed-app.yaml"),
		"-f", filepath.Join(l.Root, "harness.yaml"),
	}
	if !slices.Equal(args, want) {
		t.Fatalf("args\n%q\nwant\n%q", args, want)
	}
	decl.Env.Outputs = []contract.RelPath{".env"}
	args, err = ComposeArgs(runID, l, decl)
	if err != nil || slices.Contains(args, filepath.Join(l.Src, ".env")) || !slices.Contains(args, filepath.Join(l.Src, "backend", "app")) {
		t.Fatalf("a root .env with a nested project directory: %q %v", args, err)
	}
}

func TestRelativeRunDirectoryIsRejected(t *testing.T) {
	for _, dir := range []string{"runs/r1", "./r1", ""} {
		if _, err := NewLayout(dir); err == nil {
			t.Errorf("%q was accepted", dir)
		}
	}
	l, err := NewLayout("/state/runs/r1/")
	if err != nil || l.Src != "/state/runs/r1/src" || l.Work != "/state/runs/r1/work" || l.Build != "/state/runs/r1/build" {
		t.Fatalf("layout %+v %v", l, err)
	}
}

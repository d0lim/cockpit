package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/runnerbin"
)

func call(env map[string]string, args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, func(k string) string { return env[k] }, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersionFlagPrintsHubPrefixAndVersion(t *testing.T) {
	code, out, errOut := call(nil, "--version")
	if code != 0 || !strings.HasPrefix(out, "cockpit ") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
}

func TestNoArgsUnknownCommandOrIncompleteSubcommandPrintsUsageAndExits2(t *testing.T) {
	for _, args := range [][]string{nil, {"fly"}, {"sdk"}, {"sdk", "write"}, {"sdk", "read", "x"}, {"runner-image", "extra"}} {
		code, _, errOut := call(nil, args...)
		if code != 2 || !strings.Contains(errOut, "usage") {
			t.Fatalf("%v: exit %d stderr %q", args, code, errOut)
		}
	}
}

func TestSdkWriteWritesDeclarationsToDirAndPrintsOnePathPerLine(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sdk")
	code, out, errOut := call(nil, "sdk", "write", dir)
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 4 {
		t.Fatalf("%v %v", entries, err)
	}
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 4 || !strings.HasSuffix(lines[1], "cockpit-flow.d.ts") {
		t.Fatalf("stdout %q", out)
	}
}

func TestRunnerImagePrintsEnsuredTagOnOneLineAndExits2OnFailure(t *testing.T) {
	home := t.TempDir()
	original := ensureImage
	t.Cleanup(func() { ensureImage = original })
	var pinPath string
	ensureImage = func(_ context.Context, pins runnerbin.Pins) (runnerbin.Image, error) {
		pinPath = pins.(runnerbin.FilePins).Path
		return runnerbin.Image{Tag: "cockpit-runner:abcdef123456", ID: "sha256:1", Arch: "arm64"}, nil
	}
	code, out, errOut := call(map[string]string{"COCKPIT_HOME": home}, "runner-image")
	if code != 0 || out != "cockpit-runner:abcdef123456\n" || errOut != "" || pinPath != filepath.Join(home, "runner-images.json") {
		t.Fatalf("exit %d stdout %q stderr %q pin %q", code, out, errOut, pinPath)
	}
	ensureImage = func(context.Context, runnerbin.Pins) (runnerbin.Image, error) {
		return runnerbin.Image{}, errors.New("runner not embedded: not a release build")
	}
	code, out, errOut = call(map[string]string{"COCKPIT_HOME": home}, "runner-image")
	if code != 2 || out != "" || !strings.Contains(errOut, "runner not embedded") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
}

func TestInitScaffoldsIntoTheGivenDirectoryAndPrintsNextSteps(t *testing.T) {
	dir := t.TempDir()
	code, out, errOut := call(nil, "init", "--base-url", "http://web:8080", dir)
	if code != 0 || !strings.Contains(out, "created e2e/env.json") || !strings.Contains(out, "cockpit runner-image") || errOut != "" {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "compose.cockpit.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestInitRefusesBadArgumentsAndBadBaseURLsWithExit2(t *testing.T) {
	for _, args := range [][]string{{"init", "--force"}, {"init", "a", "b"}, {"init", "--base-url"}} {
		if code, _, errOut := call(nil, args...); code != 2 || !strings.Contains(errOut, "usage") {
			t.Fatalf("%v: exit %d stderr %q", args, code, errOut)
		}
	}
	code, _, errOut := call(nil, "init", "--base-url", "http://web/api", t.TempDir())
	if code != 2 || !strings.Contains(errOut, "not an origin") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}

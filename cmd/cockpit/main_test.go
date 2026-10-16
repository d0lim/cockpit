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

func TestVersion출력은_허브_접두와_버전을_싣는다(t *testing.T) {
	code, out, errOut := call(nil, "--version")
	if code != 0 || !strings.HasPrefix(out, "cockpit ") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
}

func Test인자_없는_호출과_모르는_명령과_불완전한_하위_명령은_usage_와_종료_2(t *testing.T) {
	for _, args := range [][]string{nil, {"fly"}, {"sdk"}, {"sdk", "write"}, {"sdk", "read", "x"}, {"runner-image", "extra"}} {
		code, _, errOut := call(nil, args...)
		if code != 2 || !strings.Contains(errOut, "usage") {
			t.Fatalf("%v: exit %d stderr %q", args, code, errOut)
		}
	}
}

func TestSdk_write_는_디렉토리에_선언_셋을_쓰고_이름을_한_줄씩_낸다(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sdk")
	code, out, errOut := call(nil, "sdk", "write", dir)
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 3 {
		t.Fatalf("%v %v", entries, err)
	}
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 3 || !strings.HasSuffix(lines[1], "cockpit-flow.d.ts") {
		t.Fatalf("stdout %q", out)
	}
}

func TestRunner_image_는_보장한_이미지_태그_한_줄을_내고_실패하면_종료_2_다(t *testing.T) {
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
		return runnerbin.Image{}, errors.New("러너 미내장 — 릴리즈 빌드가 아니다")
	}
	code, out, errOut = call(map[string]string{"COCKPIT_HOME": home}, "runner-image")
	if code != 2 || out != "" || !strings.Contains(errOut, "러너 미내장") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
}

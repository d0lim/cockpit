package headless

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/cli"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
)

var headlessRunID = regexp.MustCompile(`^run-[0-9]{8}-[0-9]{6}-[0-9a-f]{7}-h[0-9]+$`)

func TestAPassingFakeRunExitsZeroAndCopiesABundleWhoseSummarySaysPassed(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	code, text := runFake(t, toyProject(t), newFake(nil), Options{Out: out})
	if code != cli.ExitOK {
		t.Fatalf("exit %d\n%s", code, text)
	}
	summary := readJSON(t, filepath.Join(out, "summary.json"))
	meta := readJSON(t, filepath.Join(out, "meta.json"))
	id, _ := summary["runId"].(string)
	if summary["verdict"] != "passed" || !headlessRunID.MatchString(id) || meta["id"] != id || meta["submitter"] != "tester" {
		t.Fatalf("summary verdict %v id %v meta %v", summary["verdict"], summary["runId"], meta)
	}
	for _, want := range []string{"ok   seal static", "ok   report", "ok   bundle copy", "verdict passed"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q\n%s", want, text)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(text), "verdict passed") {
		t.Errorf("the verdict is not the last line\n%s", text)
	}
	files := 0
	err := filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		want := fs.FileMode(0o600)
		if d.IsDir() {
			want = 0o700
		} else {
			files++
		}
		if err == nil && info.Mode().Perm() != want {
			t.Errorf("%s mode %o", p, info.Mode().Perm())
		}
		return err
	})
	if err != nil || files < 6 {
		t.Fatalf("walk: %v, %d files", err, files)
	}
}

func TestAFailingStepExitsOneWithVerdictFailedAndItsStopPointAsAReason(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	steps, stop := failingSteps("expected status 200 but got 503")
	f := newFake(func(f *fakedocker.Fake) { f.RunnerCode, f.Steps, f.StoppedAt = 1, steps, stop })
	code, text := runFake(t, toyProject(t), f, Options{Out: out})
	if code != cli.ExitFailed {
		t.Fatalf("exit %d\n%s", code, text)
	}
	summary := readJSON(t, filepath.Join(out, "summary.json"))
	if summary["verdict"] != "failed" || !strings.Contains(text, "ok   runner") || !strings.Contains(text, "reason: health › health-ok — expected status 200 but got 503") {
		t.Fatalf("summary verdict %v\n%s", summary["verdict"], text)
	}
}

func TestASealViolationExitsOneWithVerdictFailedAndWritesNoBundleOutOrArchive(t *testing.T) {
	dir := t.TempDir()
	out, archive := filepath.Join(dir, "bundle"), filepath.Join(dir, "run.tar.gz")
	code, text := runFake(t, toyProject(t), newFake(func(f *fakedocker.Fake) { f.ProbeLeak = true }), Options{Out: out, Archive: archive})
	if code != cli.ExitFailed {
		t.Fatalf("exit %d\n%s", code, text)
	}
	for _, p := range []string{out, archive} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s exists after a run without a bundle: %v", p, err)
		}
	}
	for _, want := range []string{"FAIL seal probes", "no report bundle: nothing is written", "verdict failed, no report bundle: the run closed in the seal check stage", "reason: the seal violated"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "ok   boot") || strings.Contains(text, "report (") {
		t.Fatalf("the run went past the seal check\n%s", text)
	}
}

func TestTheArchiveIndexListsEveryBundleFileUnderTheRunID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.tar.gz")
	code, text := runFake(t, toyProject(t), newFake(nil), Options{Archive: path})
	if code != cli.ExitOK {
		t.Fatalf("exit %d\n%s", code, text)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("archive %v: %v", info, err)
	}
	entries := archiveEntries(t, path)
	var index contract.Archive
	var id string
	for name, data := range entries {
		if strings.HasSuffix(name, "/index.json") {
			id = strings.TrimSuffix(name, "/index.json")
			if err := json.Unmarshal(data, &index); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !headlessRunID.MatchString(id) || index.RunID != id || !strings.Contains(text, "ok   archive ("+"") {
		t.Fatalf("index of %q: %+v\n%s", id, index, text)
	}
	listed := map[string]bool{}
	for _, f := range index.Files {
		listed[f.Path] = true
		if _, ok := entries[id+"/"+f.Path]; !ok {
			t.Errorf("index lists %s, which the archive lacks", f.Path)
		}
	}
	for _, want := range []string{"report/summary.json", "report/meta.json", "report/not-proven.md", "report/seal.json", "report/e2e/manifest.json", "report/e2e/result.json"} {
		if !listed[want] {
			t.Errorf("index lacks %s: %v", want, listed)
		}
	}
	if len(index.Files) < 6 {
		t.Fatalf("index lists %d files", len(index.Files))
	}
}

func TestAnInterruptInTheRunStageExits130AndStillDeliversAnAbortedBundle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := filepath.Join(t.TempDir(), "bundle")
	f := newFake(func(f *fakedocker.Fake) {
		f.RunnerHangs = true
		f.OnVerb = func(verb string) {
			if verb == "wait" {
				cancel()
			}
		}
	})
	code, text := runFakeIn(ctx, t, toyProject(t), f, Options{Out: out})
	if code != ExitInterrupted {
		t.Fatalf("exit %d\n%s", code, text)
	}
	summary := readJSON(t, filepath.Join(out, "summary.json"))
	if summary["verdict"] != "aborted" || !strings.Contains(text, "FAIL runner") || !strings.Contains(text, "ok   cleanup") || !strings.Contains(text, "verdict aborted") {
		t.Fatalf("summary verdict %v\n%s", summary["verdict"], text)
	}
}

package headless

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/cli"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	hubstages "github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var redactionMutations = []porttest.Mutation{
	{"the verdict block passes the run redactor", "print the verdict block with fmt.Fprintln(r.st.out) instead of r.st.say", "red: the planted value shows in a reason, the notice and a tail line", "TestTheVerdictBlockMasksTheReasonsTheNoticeAndTheTailWithTheRunRedactor"},
	{"the report bundle and the archive use the run masker", "pass a fresh mask.New with no run values as the Masker of pipeline.RunReport in report and of archive.Input in writeArchive", "red: the planted runner line shows in the archive's summary.json", "TestAFailedHeadlessRunPrintsAndArchivesNoPlantedValue"},
	{"an existing --out or --archive path is never replaced", "return nil from outputs and drop the Lstat check in writeArchive", "red: the archive is replaced and the run exits 0", "TestAnExistingArchiveOrOutPathIsRefusedBeforeTheRunAndLeftUntouched"},
}

func TestRedactionMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "redaction_test.go", redactionMutations)
}

func TestTheVerdictBlockMasksTheReasonsTheNoticeAndTheTailWithTheRunRedactor(t *testing.T) {
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	r := &run{env: &hubstages.Env{Masker: m}}
	r.st = &stages{out: &out, mask: r.mask}
	r.meta.Verdict, r.meta.FailedStage = lifecycle.VerdictError, lifecycle.Booting
	r.meta.Notice, r.meta.Tail = "boot failed with "+planted, []string{"app said " + planted}
	r.reasons = []string{"rule reason " + planted}
	r.verdict()
	text := out.String()
	if strings.Contains(text, planted) || strings.Count(text, "****") != 3 {
		t.Fatalf("verdict block:\n%s", text)
	}
	if !strings.Contains(text, "verdict error, no report bundle: the run closed in the boot stage") {
		t.Fatalf("canary: the verdict line is missing\n%s", text)
	}
}

func TestAFailedHeadlessRunPrintsAndArchivesNoPlantedValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.tar.gz")
	steps, stop := failingSteps("token " + planted + " rejected")
	f := newFake(func(f *fakedocker.Fake) { f.RunnerCode, f.Steps, f.StoppedAt = 1, steps, stop })
	code, text := runFake(t, toyProject(t), f, Options{Archive: path})
	if code != cli.ExitFailed {
		t.Fatalf("exit %d\n%s", code, text)
	}
	if strings.Contains(text, planted) || !strings.Contains(text, "reason: health › health-ok — token ****") {
		t.Fatalf("stdout:\n%s", text)
	}
	var all strings.Builder
	entries := archiveEntries(t, path)
	for name, data := range entries {
		if !strings.Contains(name, "/schema/") {
			all.Write(data)
		}
	}
	if text := all.String(); strings.Contains(text, planted) || !strings.Contains(text, "token ****") {
		t.Fatalf("the archive holds the planted value or masked nothing (%d entries)", len(entries))
	}
}

func TestAnExistingArchiveOrOutPathIsRefusedBeforeTheRunAndLeftUntouched(t *testing.T) {
	dir := t.TempDir()
	archive, out := filepath.Join(dir, "run.tar.gz"), filepath.Join(dir, "bundle")
	if err := os.WriteFile(archive, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	project := toyProject(t)
	for _, opts := range []Options{{Archive: archive}, {Out: out}} {
		f := newFake(nil)
		code, text := runFake(t, project, f, opts)
		if code != cli.ExitRejected || len(f.Calls()) != 0 || !strings.Contains(text, "exists and is not replaced") {
			t.Errorf("%+v: exit %d, %d docker calls\n%s", opts, code, len(f.Calls()), text)
		}
	}
	data, err := os.ReadFile(archive)
	entries, _ := os.ReadDir(out)
	if err != nil || string(data) != "keep me" || len(entries) != 0 {
		t.Fatalf("the archive holds %q (%v) and the out directory %d entries", data, err, len(entries))
	}
	code, text := runFake(t, project, newFake(nil), Options{Archive: filepath.Join(dir, "fresh.tar.gz")})
	if code != cli.ExitOK || !strings.Contains(text, "ok   archive") {
		t.Fatalf("canary: a fresh archive path is refused: exit %d\n%s", code, text)
	}
}

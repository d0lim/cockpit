package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const altFile contract.DeclarationFile = ".cockpit/alt.yaml"

var declarationMutations = []porttest.Mutation{
	{"the hub accepts against the declaration the submission names", "pass an empty DeclarationFile to ReadDeclaration in Hub.Accept", "red: the alt submission runs the suite smoke", "TestASubmissionOfAnotherDeclarationRunsItsSuiteWithItsEnvAndEveryLaterReadUsesIt"},
	{"the stages read the declaration the run record names", "pass an empty DeclarationFile to ReadDeclaration in Hub.declaration", "red: work/.cockpit/project.yaml of the alt run holds the default declaration", "TestASubmissionOfAnotherDeclarationRunsItsSuiteWithItsEnvAndEveryLaterReadUsesIt"},
	{"an operation opens a run with the declaration it read", "read src/.cockpit/project.yaml in envops.Ops.Open", "red: the opened alt run declares scripts/suites", "TestASubmissionOfAnotherDeclarationRunsItsSuiteWithItsEnvAndEveryLaterReadUsesIt"},
	{"a host API refusal reads the version from the declaration the submission names", "read compose.DeclarationPath in declaredHostAPI", "red: the alt submission with hostApi 2 is refused without naming version 2", "TestASubmissionOfADeclarationTheCommitLacksOrOfAnUnservedHostAPIIsRefusedAndCreatesNoRun"},
}

func TestDeclarationMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "declaration_test.go", declarationMutations)
}

func TestASubmissionOfAnotherDeclarationRunsItsSuiteWithItsEnvAndEveryLaterReadUsesIt(t *testing.T) {
	w := newWorld(t, nil, nil)
	res := w.sched.Submit(context.Background(), scheduler.SubmitInput{Worktree: w.repo, Submitter: "tester", Declaration: altFile})
	if res.Fail != nil {
		t.Fatalf("submit: %+v", res.Fail)
	}
	w.sched.Tick()
	w.sched.Idle()
	m := w.meta(res.ID)
	if m.Suite != "alt" || m.Declaration != altFile || m.Phase != lifecycle.Done || m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("alt run %+v", m)
	}
	dir, _ := w.hub.Store().RunDir(m.ID)
	work, err := os.ReadFile(filepath.Join(dir, "work", compose.DeclarationPath))
	if err != nil || !strings.Contains(string(work), "suites: scripts/alt") {
		t.Fatalf("work/%s of the alt run:\n%s\n%v", compose.DeclarationPath, work, err)
	}
	harness, err := os.ReadFile(filepath.Join(dir, "harness.yaml"))
	if err != nil || !strings.Contains(string(harness), "E2E_ENV: /work/scripts/env.alt.json") || !strings.Contains(string(harness), "COCKPIT_SUITES: /work/scripts/alt") {
		t.Fatalf("the harness of the alt run lacks E2E_ENV or COCKPIT_SUITES: %v\n%s", err, harness)
	}
	r, err := w.hub.ops.Open(m.ID)
	if err != nil || r.Decl == nil || r.Decl.Scripts.Suites != "scripts/alt" {
		t.Fatalf("the opened alt run: %+v, %v", r, err)
	}
	r.Close()
}

func TestASubmissionOfADeclarationTheCommitLacksOrOfAnUnservedHostAPIIsRefusedAndCreatesNoRun(t *testing.T) {
	w := newWorld(t, nil, func(dir string) { replaceIn(t, filepath.Join(dir, altFile.Path()), "hostApi: 1", "hostApi: 2") })
	submit := func(file contract.DeclarationFile) scheduler.SubmitResult {
		return w.sched.Submit(context.Background(), scheduler.SubmitInput{Worktree: w.repo, Submitter: "tester", Declaration: file})
	}
	missing := submit(".cockpit/other.yaml")
	if missing.Fail == nil || missing.Fail.Code != ticker.CodeRejectedNoHarness || !strings.Contains(missing.Fail.Reason, ".cockpit/other.yaml") {
		t.Fatalf("a declaration the commit lacks: %+v", missing)
	}
	unserved := submit(altFile)
	if unserved.Fail == nil || unserved.Fail.Code != ticker.CodeRejectedHostAPI || !strings.Contains(unserved.Fail.Reason, "unknown host API version 2:") {
		t.Fatalf("an alt declaration with hostApi 2: %+v", unserved)
	}
	list, err := w.hub.Store().ListRuns()
	if err != nil || len(list.Runs) != 0 {
		t.Fatalf("refusals created runs: %d, %v", len(list.Runs), err)
	}
	if served := submit(""); served.Fail != nil {
		t.Fatalf("the default declaration was refused: %+v", served.Fail)
	}
}

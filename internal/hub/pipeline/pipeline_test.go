package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var pipelineMutations = []porttest.Mutation{
	{"stages run in the documented order", "swap build and seal check in stageFor", "red: the stage lines or docker calls come out of order", "TestHappyRunWalksPrepareBuildSealBootRunReportAndPassesWithThePinnedImage"},
	{"a build failure closes as error without an environment", "let build return Built on a build error", "red: the run reaches booting or keeps its src tree", "TestBuildFailureSealViolationAndBootFailureCloseAsErrorFailedAndErrorWithTheDocumentedEnvironment"},
	{"a seal violation closes as failed", "drop Violation from the probe failure", "red: the leaking run closes as error", "TestBuildFailureSealViolationAndBootFailureCloseAsErrorFailedAndErrorWithTheDocumentedEnvironment"},
	{"a boot failure stops the environment", "skip StopQuietly in settle", "red: no compose stop after the failed boot", "TestBuildFailureSealViolationAndBootFailureCloseAsErrorFailedAndErrorWithTheDocumentedEnvironment"},
}

func TestPipelineMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "pipeline_test.go", pipelineMutations)
}

func TestHappyRunWalksPrepareBuildSealBootRunReportAndPassesWithThePinnedImage(t *testing.T) {
	w := newWorld(t, nil, nil)
	m := w.submit()
	if m.Phase != lifecycle.Done || m.Env != lifecycle.EnvUp || m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	if m.RunnerImage == nil || m.RunnerImage.Tag != fakedocker.RunnerTag || m.RunnerImage.ID != fakedocker.PinnedID || m.ImageKeys["IMG_APP"] == "" {
		t.Fatalf("pins %+v keys %v", m.RunnerImage, m.ImageKeys)
	}
	events := w.eventText()
	stagesSeen := []string{"stage prepare", "stage build", "stage seal check", "stage boot", "stage run", "stage report"}
	last := -1
	for _, s := range stagesSeen {
		at := strings.Index(events, s)
		if at <= last {
			t.Fatalf("stage %q out of order in\n%s", s, events)
		}
		last = at
	}
	verbs := w.verbs()
	order := []string{"run", "compose config", "compose run builder", "compose build", "compose run probe", "compose run probe-control", "compose up", "compose ps", "image inspect", "compose run runner", "wait", "logs", "inspect", "rm"}
	last = -1
	for _, v := range order {
		at := indexOf(verbs[last+1:], v)
		if at < 0 {
			t.Fatalf("%q missing after position %d in %v", v, last, verbs)
		}
		last += at + 1
	}
	dir, _ := w.hub.Store().RunDir(m.ID)
	if _, err := os.Stat(filepath.Join(dir, "seal.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "build")); !os.IsNotExist(err) {
		t.Fatalf("build/ was kept: %v", err)
	}
	if m.Pgid != nil {
		t.Fatalf("pgid %d left after the run", *m.Pgid)
	}
}

func TestBuildFailureSealViolationAndBootFailureCloseAsErrorFailedAndErrorWithTheDocumentedEnvironment(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*fakedocker.Fake)
		verdict lifecycle.Verdict
		env     lifecycle.Env
		stage   lifecycle.Phase
	}{
		{"build failure", func(f *fakedocker.Fake) { f.FailVerb = map[string]bool{"compose build": true} }, lifecycle.VerdictError, lifecycle.EnvAbsent, lifecycle.Building},
		{"seal violation", func(f *fakedocker.Fake) { f.ProbeLeak = true }, lifecycle.VerdictFailed, lifecycle.EnvAbsent, lifecycle.Sealing},
		{"boot failure", func(f *fakedocker.Fake) { f.AppExited = true }, lifecycle.VerdictError, lifecycle.EnvStopped, lifecycle.Booting},
	}
	evaluated := 0
	for _, c := range cases {
		w := newWorld(t, c.mutate, nil)
		m := w.submit()
		if m.Phase != lifecycle.Done || m.Verdict != c.verdict || m.Env != c.env || m.FailedStage != c.stage {
			t.Errorf("%s: state %+v failed stage %s notice %q", c.name, m.State, m.FailedStage, m.Notice)
		}
		dir, _ := w.hub.Store().RunDir(m.ID)
		_, srcErr := os.Stat(filepath.Join(dir, "src"))
		st, _ := w.hub.Store().CurrentState()
		_, leased := st.Leases[m.ID]
		verbs := w.verbs()
		switch c.env {
		case lifecycle.EnvAbsent:
			if !os.IsNotExist(srcErr) || leased || indexOf(verbs, "compose down") < 0 {
				t.Errorf("%s: src kept %v, leased %v, down %d", c.name, srcErr, leased, indexOf(verbs, "compose down"))
			}
		case lifecycle.EnvStopped:
			if srcErr != nil || !leased || indexOf(verbs, "compose stop") < 0 {
				t.Errorf("%s: src %v, leased %v, stop %d", c.name, srcErr, leased, indexOf(verbs, "compose stop"))
			}
		}
		if indexOf(verbs, "compose run runner") >= 0 {
			t.Errorf("%s: the runner ran", c.name)
		}
		evaluated++
	}
	if evaluated != 3 {
		t.Fatalf("evaluated %d", evaluated)
	}
}

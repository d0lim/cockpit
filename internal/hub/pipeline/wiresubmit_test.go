package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/seal"
)

const wireSection = `wire:
  routes:
    - { name: app, service: app, port: 8080 }
  publicFields:
    - { key: token, value: "tok-[0-9a-f]{8,64}" }
`

var wireSubmitMutations = []porttest.Mutation{
	{"a commit without routes cannot record", "drop the wire.routes check from Hub.Accept", "red: the submission is accepted and creates a run", "TestAWireSubmissionForACommitWithoutRoutesIsRefusedAndCreatesNoRun"},
	{"the harness and run env of a recording run carry the recorder", "stop passing Wire to HarnessInput in WriteRunFiles", "red: roles.present fails because the harness has no recorder", "TestARunSubmittedWithWireRendersTheRecorderPassesTheSealAndBoots"},
	{"the seal knows that the run records", "stop passing Wire to seal.Input in SealStatic", "red: wire.routes fails on the recorder of a recording run", "TestARunSubmittedWithWireRendersTheRecorderPassesTheSealAndBoots"},
	{"a recording run needs its recorder in the rendered config", "stop passing Wire to seal.Input in SealStatic", "red: the run without a rendered recorder passes the seal", "TestARecordingRunWhoseConfigLacksTheRecorderFailsTheStaticSeal"},
	{"a run without the option records nothing", "set Wire on every run in Hub.begin", "red: the harness of the plain run holds the recorder", "TestARunWithoutTheOptionRendersNoRecorderEvenWhenTheCommitDeclaresRoutes"},
}

func TestWireSubmitMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "wiresubmit_test.go", wireSubmitMutations)
}

func declareWire(t *testing.T) func(dir string) {
	return func(dir string) {
		t.Helper()
		path := filepath.Join(dir, ".cockpit", "project.yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, wireSection...), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func (w *world) submitWire() (id string, fail *ticker.Failure) {
	w.t.Helper()
	res := w.sched.Submit(context.Background(), scheduler.SubmitInput{Worktree: w.repo, Submitter: "tester", Wire: true})
	if res.Fail != nil {
		return "", res.Fail
	}
	w.sched.Tick()
	w.sched.Idle()
	return res.ID, nil
}

type sealEntry struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Actual string `json:"actual"`
}

func staticResult(t *testing.T, dir, name string) sealEntry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, seal.FileName))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Static []sealEntry `json:"static"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	for _, r := range report.Static {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("the seal report has no %s among %d assertions", name, len(report.Static))
	return sealEntry{}
}

func TestARunSubmittedWithWireRendersTheRecorderPassesTheSealAndBoots(t *testing.T) {
	w := newWorld(t, nil, declareWire(t))
	id, fail := w.submitWire()
	if fail != nil {
		t.Fatalf("submit: %+v", fail)
	}
	m := w.meta(id)
	if !m.Wire || m.Verdict != lifecycle.VerdictPassed || m.Env != lifecycle.EnvUp {
		t.Fatalf("meta wire %v state %+v notice %q\n%s", m.Wire, m.State, m.Notice, w.eventText())
	}
	dir, _ := w.hub.Store().RunDir(id)
	harness, err := os.ReadFile(filepath.Join(dir, compose.HarnessFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{compose.RoleWire + ":", "41001=app:8080", "--public-field", "COCKPIT_WIRE_ROUTES: app=app:8080=cockpit-wire:41001"} {
		if !strings.Contains(string(harness), want) {
			t.Errorf("the harness lacks %q:\n%s", want, harness)
		}
	}
	env, err := os.ReadFile(filepath.Join(dir, compose.RunEnvFile))
	if err != nil || !strings.Contains(string(env), "COCKPIT_WIRE_APP=cockpit-wire:41001\n") {
		t.Fatalf("run.env %v:\n%s", err, env)
	}
	if r := staticResult(t, dir, "wire.routes"); !r.OK {
		t.Fatalf("wire.routes: %+v", r)
	}
	if indexOf(w.verbs(), "compose up") < 0 {
		t.Fatalf("the environment never came up: %v", w.verbs())
	}
}

func TestARecordingRunWhoseConfigLacksTheRecorderFailsTheStaticSeal(t *testing.T) {
	w := newWorld(t, func(f *fakedocker.Fake) { f.Wire.Unrendered = true }, declareWire(t))
	id, fail := w.submitWire()
	if fail != nil {
		t.Fatalf("submit: %+v", fail)
	}
	m := w.meta(id)
	if m.Verdict != lifecycle.VerdictFailed || !strings.Contains(m.Notice, "no "+compose.RoleWire+" service") {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	if indexOf(w.verbs(), "compose up") >= 0 {
		t.Fatalf("a run without its recorder came up: %v", w.verbs())
	}
	plain := newWorld(t, func(f *fakedocker.Fake) { f.Wire.Unrendered = true }, declareWire(t))
	if m := plain.submit(); m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("canary: a plain run needs no recorder: %+v %q", m.State, m.Notice)
	}
}

func TestAWireSubmissionForACommitWithoutRoutesIsRefusedAndCreatesNoRun(t *testing.T) {
	w := newWorld(t, nil, nil)
	_, fail := w.submitWire()
	if fail == nil || fail.Status != 400 || fail.Code != ticker.CodeRejectedNoHarness || !strings.Contains(fail.Reason, "declares no wire.routes") {
		t.Fatalf("failure %+v", fail)
	}
	list, err := w.hub.Store().ListRuns()
	if err != nil || len(list.Runs) != 0 {
		t.Fatalf("runs %+v %v", list.Runs, err)
	}
	if m := w.submit(); m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("canary: the same commit without wire is accepted: %+v %q", m.State, m.Notice)
	}
}

func TestARunWithoutTheOptionRendersNoRecorderEvenWhenTheCommitDeclaresRoutes(t *testing.T) {
	w := newWorld(t, nil, declareWire(t))
	m := w.submit()
	if m.Wire || m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("meta wire %v state %+v notice %q", m.Wire, m.State, m.Notice)
	}
	dir, _ := w.hub.Store().RunDir(m.ID)
	harness, err := os.ReadFile(filepath.Join(dir, compose.HarnessFile))
	if err != nil || !strings.Contains(string(harness), compose.RoleRunner) {
		t.Fatalf("harness %v:\n%s", err, harness)
	}
	if strings.Contains(string(harness), compose.RoleWire) {
		t.Fatalf("the harness of a plain run names the recorder:\n%s", harness)
	}
	env, err := os.ReadFile(filepath.Join(dir, compose.RunEnvFile))
	if err != nil || !strings.Contains(string(env), "RUN_ID=") || strings.Contains(string(env), "COCKPIT_WIRE_") {
		t.Fatalf("run.env %v:\n%s", err, env)
	}
	if r := staticResult(t, dir, "wire.routes"); !r.OK {
		t.Fatalf("wire.routes: %+v", r)
	}
}

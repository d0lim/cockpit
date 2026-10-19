package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/seal"
)

var edgesMutations = []porttest.Mutation{
	{"a failed extraction releases the lease", "skip CloseWithoutEnv in settle", "red: the lease outlives the failed extraction", "TestATreeExtractionFailureClosesAsErrorWithoutAnEnvironmentReleasesTheLeaseAndRunsNoContainer"},
	{"the static seal runs before the build", "skip SealStatic in prepare", "red: the run passes and the build runs", "TestAStaticSealViolationClosesAsFailedBeforeTheBuildAndLeavesSealJSON"},
	{"ports are checked again before the seal", "drop the BlockedPorts refusal in sealStage", "red: the run passes with the port taken", "TestAPortTakenBeforeTheSealClosesAsErrorNamingThePortWithoutLeasingAgain"},
	{"a seal without a working control is not proven", "return SealPassed for an inconclusive seal in seal.Verdict", "red: the run boots without a proven seal", "TestASealWhoseControlCannotSucceedClosesAsErrorAndBringsTheProjectDown"},
	{"a failed report notice is masked", "build the report failed notice in reportStage from the raw error", "red: the planted value shows in the notice", "TestAFailedReportClosesAsErrorKeepsTheEnvironmentUpAndMasksTheCredentialInItsNotice"},
	{"a failed stage's output is masked", "keep the raw stderr in compose.Failed and the raw error in the pipeline notice, both layers together", "red: the planted value shows in the notice", "TestAStageFailingWithACredentialInItsOutputLeavesOnlyTheMaskedFormInTheNoticeAndTail"},
	{"a runner that cannot start leaves the environment up", "stop the environment in settle when a run with its environment up closes as error", "red: compose stop runs and the env is stopped", "TestARunnerThatFailsToStartClosesAsErrorWithTheEnvironmentStillUp"},
}

func TestEdgesMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "edges_test.go", edgesMutations)
}

func (w *world) submitWith(before func(id string)) store.Meta {
	w.t.Helper()
	res := w.sched.Submit(context.Background(), scheduler.SubmitInput{Worktree: w.repo, Submitter: "tester"})
	if res.Fail != nil {
		w.t.Fatalf("submit: %+v", res.Fail)
	}
	if before != nil {
		before(res.ID)
	}
	w.sched.Tick()
	w.sched.Idle()
	return w.meta(res.ID)
}

func (w *world) leased(id string) bool {
	st, _ := w.hub.Store().CurrentState()
	_, ok := st.Leases[id]
	return ok
}

func closedState(t *testing.T, m store.Meta, verdict lifecycle.Verdict, env lifecycle.Env, stage lifecycle.Phase) {
	t.Helper()
	if m.Phase != lifecycle.Done || m.Verdict != verdict || m.Env != env || m.FailedStage != stage {
		t.Fatalf("state %+v failed stage %q notice %q, want %s/%s at %s", m.State, m.FailedStage, m.Notice, verdict, env, stage)
	}
}

func TestATreeExtractionFailureClosesAsErrorWithoutAnEnvironmentReleasesTheLeaseAndRunsNoContainer(t *testing.T) {
	w := newWorld(t, nil, nil)
	m := w.submitWith(func(id string) {
		dir, _ := w.hub.Store().RunDir(id)
		if err := os.WriteFile(filepath.Join(dir, "src"), []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	closedState(t, m, lifecycle.VerdictError, lifecycle.EnvAbsent, lifecycle.Preparing)
	if !strings.Contains(m.Notice, "cannot create src") || w.leased(m.ID) {
		t.Fatalf("notice %q, leased %v", m.Notice, w.leased(m.ID))
	}
	for _, v := range w.verbs() {
		if strings.HasPrefix(v, "compose") || v == "run" {
			t.Fatalf("docker %q ran after a failed extraction: %v", v, w.verbs())
		}
	}
}

func violatingFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "seal", "testdata", "config.toy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	app := doc["services"].(map[string]any)["app"].(map[string]any)
	app["ports"] = []any{map[string]any{"mode": "ingress", "target": 8080, "published": "22050", "protocol": "tcp", "host_ip": "0.0.0.0"}}
	out, _ := json.Marshal(doc)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAStaticSealViolationClosesAsFailedBeforeTheBuildAndLeavesSealJSON(t *testing.T) {
	fixture := violatingFixture(t)
	w := newWorld(t, func(f *fakedocker.Fake) { f.ConfigFixture = fixture }, nil)
	m := w.submit()
	closedState(t, m, lifecycle.VerdictFailed, lifecycle.EnvAbsent, lifecycle.Preparing)
	if !strings.Contains(m.Notice, "static seal violated: services.ports") || indexOf(w.verbs(), "compose build") >= 0 || indexOf(w.verbs(), "compose run builder") >= 0 {
		t.Fatalf("the build ran after a static violation: %v", w.verbs())
	}
	dir, _ := w.hub.Store().RunDir(m.ID)
	data, err := os.ReadFile(filepath.Join(dir, seal.FileName))
	var doc struct {
		Verdict string `json:"verdict"`
	}
	if err != nil || json.Unmarshal(data, &doc) != nil || doc.Verdict != "failed" {
		t.Fatalf("seal.json %q %v", data, err)
	}
}

func TestAPortTakenBeforeTheSealClosesAsErrorNamingThePortWithoutLeasingAgain(t *testing.T) {
	var taken atomic.Bool
	var w *world
	w = newWorldWith(t, func(o *Options) {
		o.PortProbe = func(p int) bool { return !(taken.Load() && p == w.base) }
	}, nil, nil)
	w.onEnsure = func(context.Context) error { taken.Store(true); return nil }
	m := w.submit()
	closedState(t, m, lifecycle.VerdictError, lifecycle.EnvAbsent, lifecycle.Sealing)
	if !strings.Contains(m.Notice, strconv.Itoa(w.base)) || !strings.Contains(m.Notice, "not leased again") {
		t.Fatalf("notice %q", m.Notice)
	}
	if indexOf(w.verbs(), "compose run probe") >= 0 || m.Slot == nil || *m.Slot != 0 {
		t.Fatalf("probes ran or the block moved: slot %v verbs %v", m.Slot, w.verbs())
	}
}

func TestASealWhoseControlCannotSucceedClosesAsErrorAndBringsTheProjectDown(t *testing.T) {
	var w *world
	w = newWorldWith(t, func(o *Options) {
		o.ListenCanary = func(int) (*seal.Canary, error) {
			c, err := seal.ListenCanary(0)
			closed, lerr := net.Listen("tcp", "127.0.0.1:0")
			if lerr == nil {
				w.fake.SetCanaryPort(closed.Addr().(*net.TCPAddr).Port)
				closed.Close()
			}
			return c, err
		}
	}, nil, nil)
	m := w.submit()
	closedState(t, m, lifecycle.VerdictError, lifecycle.EnvAbsent, lifecycle.Sealing)
	if !strings.Contains(m.Notice, "seal could not be checked") || indexOf(w.verbs(), "compose down") < 0 || indexOf(w.verbs(), "compose up") >= 0 {
		t.Fatalf("notice %q verbs %v", m.Notice, w.verbs())
	}
}

func TestAFailedReportClosesAsErrorKeepsTheEnvironmentUpAndMasksTheCredentialInItsNotice(t *testing.T) {
	w := newWorldWith(t, func(o *Options) {
		o.Reporter = func(context.Context, envops.ReportInput) (reconcile.ReportOutcome, string, error) {
			return reconcile.ReportOutcome{}, "", errors.New("cannot write the bundle with token " + planted)
		}
	}, nil, nil)
	m := w.submit()
	closedState(t, m, lifecycle.VerdictError, lifecycle.EnvUp, lifecycle.Reporting)
	if !strings.HasPrefix(m.Notice, "report failed: ") || strings.Contains(m.Notice, planted) || !strings.Contains(m.Notice, "****") {
		t.Fatalf("notice %q", m.Notice)
	}
	if indexOf(w.verbs(), "compose stop") >= 0 || indexOf(w.verbs(), "compose down") >= 0 {
		t.Fatalf("a failed report took the environment down: %v", w.verbs())
	}
}

func TestAStageFailingWithACredentialInItsOutputLeavesOnlyTheMaskedFormInTheNoticeAndTail(t *testing.T) {
	w := newWorld(t, func(f *fakedocker.Fake) {
		f.FailVerb, f.FailStderr = map[string]bool{"compose build": true}, "\nlogin with "+planted+" refused"
	}, nil)
	m := w.submit()
	closedState(t, m, lifecycle.VerdictError, lifecycle.EnvAbsent, lifecycle.Building)
	text := m.Notice + "\n" + strings.Join(m.Tail, "\n")
	if strings.Contains(text, planted) || !strings.Contains(text, "****") || !strings.Contains(text, "compose build failed on purpose") {
		t.Fatalf("notice %q tail %q", m.Notice, m.Tail)
	}
}

func TestARunnerThatFailsToStartClosesAsErrorWithTheEnvironmentStillUp(t *testing.T) {
	w := newWorld(t, func(f *fakedocker.Fake) { f.FailVerb = map[string]bool{"compose run runner": true} }, nil)
	m := w.submit()
	closedState(t, m, lifecycle.VerdictError, lifecycle.EnvUp, lifecycle.Testing)
	if !w.leased(m.ID) || indexOf(w.verbs(), "compose stop") >= 0 {
		t.Fatalf("leased %v verbs %v", w.leased(m.ID), w.verbs())
	}
}

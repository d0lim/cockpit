package scheduler

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var schedulerHostAPIMutations = []porttest.Mutation{
	{"a submission for an unserved host API creates no run", "drop the Serves check in Submit", "red: a run is written for host API 2", "TestSubmissionPinnedToAnUnservedHostAPIIsRefusedWithTheServedSetAndCreatesNoRun"},
	{"resume refuses a run pinned to an unserved host API", "drop the unserved check in Resume", "red: the stopped run starts", "TestRunPinnedToAnUnservedHostAPIIsListedAndDestroyedButResumeAndExecAreRefused"},
	{"exec refuses a run pinned to an unserved host API", "drop the unserved check in Exec", "red: the runner is called", "TestRunPinnedToAnUnservedHostAPIIsListedAndDestroyedButResumeAndExecAreRefused"},
	{"allowed events drop resume for an unserved run", "pass served=true to AllowedEvents", "red: resume is offered", "TestRunPinnedToAnUnservedHostAPIIsListedAndDestroyedButResumeAndExecAreRefused"},
	{"a served run still resumes", "refuse every resume", "red: the served run stays stopped", "TestRunPinnedToAServedHostAPIResumes"},
}

func pinned(api int) func(*store.Meta) { return func(m *store.Meta) { m.HostAPI = api } }

func TestSubmissionPinnedToAnUnservedHostAPIIsRefusedWithTheServedSetAndCreatesNoRun(t *testing.T) {
	s, w := setup(t, nil, func(d *Deps) {
		d.Accept = func(_ context.Context, in SubmitInput) (Accepted, error) {
			return Accepted{SHA: shaOf(in.Commit), HostAPI: 2}, nil
		}
	})
	r := s.Submit(ctx, sub("c1"))
	if r.Fail == nil || r.Fail.Status != 400 || r.Fail.Code != ticker.CodeRejectedHostAPI {
		t.Fatalf("%+v", r)
	}
	for _, want := range []string{"unknown host API version 2", "serves host API 1", "cockpit v0.3.0"} {
		if !strings.Contains(r.Fail.Reason, want) {
			t.Fatalf("%q lacks %q", r.Fail.Reason, want)
		}
	}
	if list, _ := w.store.ListRuns(); len(list.Runs) != 0 {
		t.Fatalf("runs %d", len(list.Runs))
	}
	s2, _ := setup(t, nil)
	if ok := s2.Submit(ctx, sub("c1")); ok.Fail != nil {
		t.Fatalf("canary: served submission refused: %+v", ok.Fail)
	}
}

func TestRunPinnedToAnUnservedHostAPIIsListedAndDestroyedButResumeAndExecAreRefused(t *testing.T) {
	s, w := setup(t, []store.Meta{preserved(1, 10, pinned(2), func(m *store.Meta) { m.Env = lifecycle.EnvStopped }), preserved(2, 11, pinned(2))})
	before := len(w.snapshot())
	resumed := s.Resume(rid(1), Actor{By: "cli"})
	execd := s.Exec(rid(2), "health", Actor{By: "cli"})
	for _, f := range []*ticker.Failure{resumed.Fail, execd.Fail} {
		if f == nil || f.Status != 409 || f.Code != ticker.CodeConflictState || !strings.Contains(f.Reason, "unknown host API version 2") {
			t.Fatalf("%+v", f)
		}
	}
	if got := w.snapshot()[before:]; len(got) != 0 {
		t.Fatalf("docker or runner called: %v", got)
	}
	list, _ := s.List()
	if len(list.Runs) != 2 || read(t, w, rid(1)).Env != lifecycle.EnvStopped || !slices.Equal(list.Served, []int{1}) {
		t.Fatalf("%+v", list.Runs)
	}
	if got := s.AllowedEvents(read(t, w, rid(1))); slices.Contains(got, lifecycle.Resume) || !slices.Contains(got, lifecycle.Destroy) {
		t.Fatalf("%v", got)
	}
	if r := s.Destroy(rid(1), Actor{By: "cli"}); !r.OK() || read(t, w, rid(1)).Env != lifecycle.EnvDestroyed {
		t.Fatalf("%+v", r.Fail)
	}
}

func TestRunPinnedToAServedHostAPIResumes(t *testing.T) {
	s, w := setup(t, []store.Meta{preserved(1, 10, func(m *store.Meta) { m.Env = lifecycle.EnvStopped })})
	if r := s.Resume(rid(1), Actor{By: "cli"}); !r.OK() || read(t, w, rid(1)).Env != lifecycle.EnvUp {
		t.Fatalf("%+v", r.Fail)
	}
	if got := s.AllowedEvents(preserved(2, 1, func(m *store.Meta) { m.Env = lifecycle.EnvStopped })); !slices.Contains(got, lifecycle.Resume) {
		t.Fatalf("%v", got)
	}
}

func TestSchedulerHostAPIMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "hostapi_test.go", schedulerHostAPIMutations)
}

package screen

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/sse"
	"github.com/d0lim/cockpit/internal/hub/store"
)

func finished(f *fixture) {
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictFailed}, doneUp))
	writeBundle(f.t, f.runDir(rid(1)))
}

func TestEveryRunScreenAnswersWithItsRegionsAndMarksItsTab(t *testing.T) {
	f := newFixture(t)
	finished(f)
	base := "/runs/" + rid(1)
	cases := []struct{ path, tab, region string }{
		{base, "Overview", "run-overview"},
		{base + "/report", "Report", "report"},
		{base + "/report/alpha", "Report", "flow"},
		{base + "/signals/trace", "Signals", "signals"},
		{base + "/signals/events", "Signals", "collectors"},
		{base + "/env", "Environment", "env"},
		{base + "/pipeline", "Pipeline", "pipeline"},
	}
	for _, c := range cases {
		status, header, body := f.get(c.path)
		if status != http.StatusOK || header.Get("Content-Security-Policy") != csp {
			t.Fatalf("%s: %d", c.path, status)
		}
		for _, want := range []string{`id="rail"`, `id="run-head"`, `id="` + c.region + `"`, `aria-current="page">` + c.tab + `<`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: missing %s", c.path, want)
			}
		}
	}
	if len(cases) != 7 {
		t.Fatal("case count")
	}
}

func TestRunScreensRefuseUnknownRunsFlowsAndViewsAndHideUnservedOnes(t *testing.T) {
	f := newFixture(t)
	finished(f)
	f.seed(runMeta(2, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvAbsent, Verdict: lifecycle.VerdictError}, func(m *store.Meta) { m.HostAPI = 99 }))
	cases := map[string]int{
		"/runs/not-a-run":                     http.StatusBadRequest,
		"/runs/" + rid(9):                     http.StatusNotFound,
		"/runs/" + rid(1) + "/report/ghost":   http.StatusNotFound,
		"/runs/" + rid(1) + "/signals/charts": http.StatusNotFound,
		"/runs/" + rid(2) + "/report":         http.StatusNotFound,
		"/runs/" + rid(2) + "/signals/trace":  http.StatusNotFound,
		"/runs/" + rid(2) + "/env":            http.StatusNotFound,
		"/runs/" + rid(2):                     http.StatusOK,
		"/runs/" + rid(2) + "/pipeline":       http.StatusOK,
		"/runs/" + rid(1) + "/report/alpha":   http.StatusOK,
	}
	for path, want := range cases {
		if status, _, _ := f.get(path); status != want {
			t.Errorf("%s: %d, want %d", path, status, want)
		}
	}
	_, _, body := f.get("/runs/" + rid(2))
	if strings.Contains(body, "/runs/"+rid(2)+"/report\"") || !strings.Contains(body, "does not serve host API 99") {
		t.Fatal("an unserved run links to its report or does not say why")
	}
	if len(cases) != 10 {
		t.Fatal("case count")
	}
}

func TestTheReportShowsTheVerdictTheFlowsWhatWasNotCountedAndFailedCollection(t *testing.T) {
	f := newFixture(t)
	finished(f)
	_, _, body := f.get("/runs/" + rid(1) + "/report")
	for _, want := range []string{
		"verdict Failed", "seal passed", "Steps 6 of 6", "failed steps: 1, the first at alpha › a2",
		`href="/runs/` + rid(1) + `/report/alpha?step=a2"`, `href="/runs/` + rid(1) + `/report/alpha"`,
		"skipped step · alpha › a3", "Collection failed · pulse deadline",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestOverviewActionsFollowTheAllowedEventsAndAnActionAnswersWithANotice(t *testing.T) {
	f := newFixture(t)
	finished(f)
	_, _, body := f.get("/runs/" + rid(1))
	enabled := func(op string) bool {
		i := strings.Index(body, `/do/`+op+`"`)
		if i < 0 {
			t.Fatalf("no %s form", op)
		}
		end := strings.Index(body[i:], "</form>")
		return !strings.Contains(body[i:i+end], "disabled")
	}
	if !enabled("stop") || enabled("resume") || !enabled("destroy") || enabled("abort") || !enabled("keep") {
		t.Fatal("the enabled actions do not follow the allowed events of a finished run that is up")
	}
	if !strings.Contains(body, `name="force" value="1"`) || !strings.Contains(body, "hx-confirm=") {
		t.Fatal("a run submitted by the CLI does not ask before it forces")
	}
	status, answer := f.post("/runs/"+rid(1)+"/do/keep", nil)
	if status != http.StatusOK || !strings.Contains(answer, "keep is on") || !f.sched.Get(rid(1)).Keep {
		t.Fatalf("keep: %d %q", status, answer)
	}
	status, answer = f.post("/runs/"+rid(1)+"/do/resume", nil)
	if status != http.StatusConflict || !strings.Contains(answer, "CONFLICT") {
		t.Fatalf("resume of an up run: %d %q", status, answer)
	}
	if status, _ := f.post("/runs/"+rid(1)+"/do/explode", nil); status != http.StatusNotFound {
		t.Fatalf("unknown action %d", status)
	}
}

func TestThePipelineShowsTheJobsOfThisRunNewestFirstAndFallsBackToTheKeptTail(t *testing.T) {
	f := newFixture(t)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictPassed}, doneUp, func(m *store.Meta) { m.Tail = []string{"kept tail line"} }))
	_, _, before := f.get("/runs/" + rid(1) + "/pipeline")
	if !strings.Contains(before, "kept tail line") {
		t.Fatal("no fallback to the kept tail")
	}
	f.bus.PublishJob(rid(1), sse.JobStarted(rid(1)+"/pipeline", "pipeline", "go"))
	f.bus.PublishJob(rid(1), sse.JobOutput(rid(1)+"/pipeline", "pipeline", "first line"))
	f.bus.PublishJob(rid(1), sse.JobExited(rid(1)+"/pipeline", "pipeline", 1))
	f.bus.PublishJob(rid(2), sse.JobOutput(rid(2)+"/pipeline", "pipeline", "another run's line"))
	_, _, body := f.get("/runs/" + rid(1) + "/pipeline")
	if !strings.Contains(body, "$ go\nfirst line") || !strings.Contains(body, "exit 1") || strings.Contains(body, "another run") || strings.Contains(body, "kept tail line") {
		t.Fatalf("pipeline:\n%s", body)
	}
}

func TestARunScreenResolvesToTheRunsRegionsWithItsQueryAndGoesAwayWithTheRun(t *testing.T) {
	f := newFixture(t)
	finished(f)
	sc, err := f.sc.Resolve(context.Background(), "/runs/"+rid(1)+"/signals/events?f=collector:x")
	if err != nil {
		t.Fatal(err)
	}
	if sc.Run != rid(1) || !slices.Equal(sc.Regions, []string{"rail", "run-head", "signals", "collectors"}) || !slices.Equal(sc.Signals, []string{"signals", "collectors"}) {
		t.Fatalf("%+v", sc)
	}
	out, err := sc.Render(context.Background(), []string{"signals"})
	if err != nil || !strings.Contains(string(out), `hx-target="#signals"`) {
		t.Fatalf("%v %s", err, out)
	}
	if _, err := f.sc.Resolve(context.Background(), "/runs/"+rid(7)); err == nil {
		t.Fatal("an unknown run resolved")
	}
}

func TestStageStepsMarkDoneNowFailedAndSkippedStages(t *testing.T) {
	states := func(r runItem) string {
		var out []string
		for _, s := range stageSteps(r.item()) {
			out = append(out, s.state)
		}
		return strings.Join(out, " ")
	}
	cases := map[string]runItem{
		"todo todo todo todo todo todo":   {phase: lifecycle.Queued},
		"done done done now todo todo":    {phase: lifecycle.Booting, started: true},
		"done done failed skip skip skip": {phase: lifecycle.Done, failed: lifecycle.Sealing, started: true},
		"done done done done done done":   {phase: lifecycle.Done, started: true},
		"skip skip skip skip skip skip":   {phase: lifecycle.Done},
	}
	for want, r := range cases {
		if got := states(r); got != want {
			t.Errorf("%+v: %q, want %q", r, got, want)
		}
	}
	if len(cases) != 5 {
		t.Fatal("case count")
	}
}

type runItem struct {
	phase   lifecycle.Phase
	failed  lifecycle.Phase
	started bool
}

func (r runItem) item() api.RunItem {
	it := api.RunItem{Phase: r.phase, FailedStage: r.failed}
	if r.started {
		it.StartedAt = "2026-10-19T01:00:01.000Z"
	}
	return it
}

package collect_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/collect/collecttest"
	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports"
	dockerport "github.com/d0lim/cockpit/internal/ports/docker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var reportMutations = []porttest.Mutation{
	{"report-time collection on a run whose environment is not up is skipped, not an error", "run the collectors whatever the environment", "red: the runner is called and errors are recorded", "TestReportTimeCollectionIsSkippedWhenTheEnvironmentIsNotUpAndRunsWhenItIsUp"},
	{"the report budget bounds the whole report", "wait for every runner without the timer", "red: Report returns only after the stuck runner", "TestScriptsOverTheReportBudgetDoNotBlockTheReportAndBecomeUnreadRecords"},
	{"budget overruns become unread records", "skip fold for unfinished collectors in finish", "red: the stuck collector has no unread record", "TestScriptsOverTheReportBudgetDoNotBlockTheReportAndBecomeUnreadRecords"},
	{"a docker denial from a script becomes an unread record of the run", "skip the out.Unread loop in record", "red: the report has no port:docker record", "TestDockerDenialFromAReportTimeScriptEndsUpInTheRunsUnreadRecords"},
}

func TestReportTimeCollectionIsSkippedWhenTheEnvironmentIsNotUpAndRunsWhenItIsUp(t *testing.T) {
	f := &fakeRunner{fn: func(context.Context, worker.Spec) (worker.Outcome, error) {
		return worker.Outcome{Status: worker.StatusFailed, Kind: "error", Failure: "no such host"}, nil
	}}
	s := newSupervisor(f, collect.Options{})
	register(t, s, collect.RunConfig{Collectors: []collect.Collector{{ID: "toy-db"}, {ID: "toy-queue", Interval: time.Hour}}})
	for _, env := range []lifecycle.Env{lifecycle.EnvStopped, lifecycle.EnvAbsent} {
		res, err := s.Report(context.Background(), supRun, env)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Collectors) != 2 || res.Collectors[0].Status != collect.ReportSkipped || res.Collectors[1].Status != collect.ReportSkipped || !strings.Contains(res.Collectors[0].Reason, string(env)) {
			t.Fatalf("%s: %+v", env, res.Collectors)
		}
		if len(res.Unread) != 0 || len(f.records("")) != 0 {
			t.Fatalf("%s: unread=%+v runs=%d", env, res.Unread, len(f.records("")))
		}
	}
	res, err := s.Report(context.Background(), supRun, lifecycle.EnvUp)
	if err != nil || res.Collectors[0].Status != collect.ReportError || len(res.Unread) != 2 || len(f.records("")) != 2 {
		t.Fatalf("canary: %+v %v", res, err)
	}
}

func TestScriptsOverTheReportBudgetDoNotBlockTheReportAndBecomeUnreadRecords(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	f := &fakeRunner{fn: func(ctx context.Context, spec worker.Spec) (worker.Outcome, error) {
		switch spec.Collector {
		case "stuck":
			<-release
			return worker.Outcome{Status: worker.StatusOK}, nil
		case "slow":
			return waitOrCancel(ctx, time.Hour), nil
		}
		return worker.Outcome{Status: worker.StatusOK}, nil
	}}
	s := newSupervisor(f, collect.Options{ReportBudget: 100 * time.Millisecond, DrainWait: 50 * time.Millisecond})
	register(t, s, collect.RunConfig{Collectors: []collect.Collector{{ID: "fine"}, {ID: "slow"}, {ID: "stuck"}}})
	start := time.Now()
	res, err := s.Report(context.Background(), supRun, lifecycle.EnvUp)
	if elapsed := time.Since(start); err != nil || elapsed > 600*time.Millisecond {
		t.Fatalf("report took %v: %v", elapsed, err)
	}
	got := map[string]collect.CollectorReport{}
	for _, c := range res.Collectors {
		got[c.ID] = c
	}
	if got["fine"].Status != collect.ReportOK || got["slow"].Status != collect.ReportError || got["stuck"].Status != collect.ReportError ||
		!strings.Contains(got["slow"].Reason, "report budget") || !strings.Contains(got["stuck"].Reason, "report budget") {
		t.Fatalf("%+v", res.Collectors)
	}
	sources := map[string]int{}
	for _, u := range res.Unread {
		sources[u.Source] += u.Count
	}
	if len(sources) != 2 || sources["collector:slow"] != 1 || sources["collector:stuck"] != 1 {
		t.Fatalf("%+v", res.Unread)
	}
}

func TestDockerDenialFromAReportTimeScriptEndsUpInTheRunsUnreadRecords(t *testing.T) {
	d := &fakeDocker{
		ps: []string{psLine("c-app", "app", supProject, supRun, ""), psLine("c-intruder", "intruder", "other", "run-9", "")},
		handle: func(_ context.Context, cmd compose.Command) (compose.Result, error) {
			_, _ = cmd.Stdout.Write([]byte("ready\n"))
			return compose.Result{}, nil
		},
	}
	dp, err := dockerport.New(dockerport.Options{Project: supProject, RunID: supRun, Docker: d})
	if err != nil {
		t.Fatal(err)
	}
	code := collecttest.Bundle(t, "toy-logs", `
		const ok = await ctx.docker.logs("app");
		if (ok.text !== "ready\n") throw new Error("app logs: " + ok.text);
		try { await ctx.docker.logs("intruder"); } catch (e) { if (e.code !== "PORT_DENIED") throw e; }
	`)
	s := newSupervisor(nil, collect.Options{ReportBudget: 10 * time.Second})
	register(t, s, collect.RunConfig{
		Collectors: []collect.Collector{{ID: "toy-logs", Code: code, Allowed: []string{ports.PortDocker}}},
		Ports:      func(string) map[string]ports.Handler { return map[string]ports.Handler{ports.PortDocker: dp} },
	})
	res, err := s.Report(context.Background(), supRun, lifecycle.EnvUp)
	if err != nil || len(res.Collectors) != 1 || res.Collectors[0].Status != collect.ReportOK {
		t.Fatalf("%+v %v", res, err)
	}
	if len(res.Unread) != 1 || res.Unread[0].Collector != "toy-logs" || res.Unread[0].Source != "port:docker" || !strings.Contains(res.Unread[0].Reason, `"intruder" has no container in this run`) {
		t.Fatalf("%+v", res.Unread)
	}
}

func TestReportMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "report_test.go", reportMutations)
}

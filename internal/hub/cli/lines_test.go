package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

func label(s string) *string { return &s }

func TestReportLinesCoverTheFourReportStatuses(t *testing.T) {
	base := api.ReportView{ID: id, PhaseLabel: "Done (up)", VerdictLabel: label("Failed")}
	stage, notice := lifecycle.Booting, "boot timed out"
	six := 6
	cases := map[string]struct {
		view api.ReportView
		want []string
	}{
		api.ReportPending:         {api.ReportView{Status: api.ReportPending}, []string{"written when the run finishes"}},
		api.ReportRedactionFailed: {api.ReportView{Status: api.ReportRedactionFailed, Files: []string{"signals/stream.jsonl"}}, []string{"redaction rescan", "  - signals/stream.jsonl"}},
		api.ReportMetaOnly:        {api.ReportView{Status: api.ReportMetaOnly, FailedStage: &stage, Notice: &notice, Tail: []string{"line"}}, []string{"stage boot", "reason: boot timed out", "  | line"}},
		api.ReportBundle: {api.ReportView{Status: api.ReportBundle, Summary: json.RawMessage(`{"reasons":["failed steps: 1"]}`), ReportDir: "/r",
			Steps:     &contract.StepCounts{Expected: &six, Actual: &six, Failed: 1},
			StoppedAt: &contract.SummaryStop{Flow: "alpha", Step: "a2", Message: "500"},
			Flows:     []contract.FlowSummary{{ID: "alpha", Stage: "basic", Status: contract.FlowOutcomeFailed, Passed: 1, Steps: 3, StoppedAt: "a2"}},
			NotProven: &contract.NotProven{Skipped: []string{"alpha › a3"}}},
			[]string{"steps 6/6 · failed 1", "stopped at: alpha › a2 — 500", "  - failed steps: 1", "  basic › alpha  failed  1/3  a2", "not proven: alpha › a3", "bundle: /r"}},
	}
	for status, c := range cases {
		v := c.view
		v.ID, v.PhaseLabel, v.VerdictLabel = base.ID, base.PhaseLabel, base.VerdictLabel
		text := strings.Join(reportLines(v), "\n")
		for _, w := range append(c.want, id+" · Done (up) · verdict Failed") {
			if !strings.Contains(text, w) {
				t.Errorf("%s: %q not in\n%s", status, w, text)
			}
		}
	}
	if len(cases) != 4 {
		t.Fatal("case count")
	}
}

func TestShowListAndExecLinesUseTheHubLabels(t *testing.T) {
	slot := 1
	detail := api.RunDetail{RunItem: api.RunItem{ID: id, Phase: lifecycle.Queued, PhaseLabel: "Queued", Position: 2, WaitReason: lifecycle.WaitActiveRun, SHA: "abc", Submitter: "cli", Slot: &slot},
		AllowedEvents: []lifecycle.Event{lifecycle.Destroy}, Services: []api.ServicePort{{Service: "app", Port: 8080, HostPort: 22101, Label: "App"}}}
	kit := &api.KitView{Kit: &api.KitFiles{Dir: "/k"}, Exec: &api.KitExec{Command: "cockpit runs exec " + id + " -- <flow id>", Flows: []string{"alpha"}}, ReportDir: "/r"}
	show := strings.Join(showLines(detail, kit), "\n")
	for _, w := range []string{"queued #2 · an active run is in progress", "actions: destroy", "127.0.0.1:22101  app:8080  App", "kit: /k", "flows that can run again: alpha", "bundle: /r"} {
		if !strings.Contains(show, w) {
			t.Errorf("show: %q not in\n%s", w, show)
		}
	}
	tickErr := "boom"
	list := strings.Join(listLines(api.HubList{Runs: []api.RunItem{detail.RunItem}, DockerDown: true, TickError: &tickErr}), "\n")
	for _, w := range []string{id + "  Queued  -", "Docker does not respond", "tick error: boom"} {
		if !strings.Contains(list, w) {
			t.Errorf("list: %q not in\n%s", w, list)
		}
	}
	code, status := 1, 500
	exec := strings.Join(execLines(api.ExecView{ExitCode: &code, TimedOut: true, Flows: []api.ExecFlow{{ID: "alpha", Steps: []api.ExecStep{{ID: "a1", Status: contract.StepStatusFailed, HTTPStatus: &status, Message: "no"}}}}}), "\n")
	if !strings.Contains(exec, "alpha › a1 › failed [500] — no") || !strings.Contains(exec, "runner exit code 1 · time limit exceeded") {
		t.Fatalf("exec\n%s", exec)
	}
}

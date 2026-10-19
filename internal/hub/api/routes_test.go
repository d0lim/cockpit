package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/report"
)

var compatItemFields = []string{"id", "phase", "env", "verdict", "mark", "sha", "suite", "submitter", "keep", "createdAt", "startedAt", "finishedAt", "lastUpAt", "slot", "position", "waitReason", "stopReason", "destroyReason", "failedStage", "notice", "recreate", "redaction", "phaseLabel", "verdictLabel"}

func properties(t *testing.T, typ reflect.Type) []string {
	t.Helper()
	var keys []string
	for k := range schemaFor(t, typ).Schema().Properties {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func writeBundle(t *testing.T, dir string) {
	t.Helper()
	bundle := filepath.Join(dir, report.DirReport)
	if err := os.MkdirAll(bundle, 0o700); err != nil {
		t.Fatal(err)
	}
	six, failed := 6, 1
	summary := contract.Summary{Format: contract.SummaryFormat3, RunID: "x", Suite: "toy", Verdict: contract.VerdictFailed, Reasons: []string{"failed steps: 1, the first at alpha › a2"},
		Steps:     contract.StepCounts{Expected: &six, Actual: &six, Failed: failed, Skipped: 3, Requests: 3},
		Flows:     []contract.FlowSummary{{ID: "alpha", Stage: "basic", Title: "Alpha", Requires: []string{}, StepList: []contract.FlowStep{}, Status: contract.FlowOutcomeFailed, Steps: 3, Passed: 1, Failed: 1, Skipped: 1, StoppedAt: "a2"}},
		StoppedAt: &contract.SummaryStop{Flow: "alpha", Step: "a2"}, NotProven: contract.NotProven{Skipped: []string{"alpha › a3"}, Declared: []string{}, Unread: []string{}},
		Seal: contract.SealPassed, Stages: map[string]contract.CollectStage{}, Ports: map[string]int{}, Images: contract.SummaryImages{Keys: map[string]string{}, Digests: map[string]string{}}, Versions: map[string]string{}}
	raw, _ := json.Marshal(summary)
	np := report.NotProvenText("x", summary.NotProven)
	for name, data := range map[string][]byte{report.FileSummary: raw, report.FileMeta: []byte(`{"id":"x"}`), report.FileNotProven: []byte(np)} {
		if err := os.WriteFile(filepath.Join(bundle, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEveryRouteAnswersWithTheSchemaOfItsResponseShape(t *testing.T) {
	f := newFixture(t, func(d *Deps) {
		d.Services = func(store.Meta) []ServicePort {
			return []ServicePort{{Name: "app", Service: "app", Port: 8080, HostPort: 22101, Label: "App", Open: contract.OpenBrowser}}
		}
	})
	status, raw := f.post("/api/runs", map[string]any{"worktree": "/w", "commit": "c1", "suite": "toy"})
	if status != http.StatusAccepted {
		t.Fatalf("submit %d %s", status, raw)
	}
	sub := validate(t, reflect.TypeFor[Submitted](), raw)
	id := sub["id"].(string)
	until(t, func() bool { v := f.sched.Get(id); return v != nil && v.Phase == lifecycle.Done })
	checked := 0
	shape := func(path string, typ reflect.Type) map[string]any {
		status, raw := f.get(path)
		if status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, raw)
		}
		checked++
		return validate(t, typ, raw)
	}
	list := shape("/api/runs", reflect.TypeFor[HubList]())
	if runs := list["runs"].([]any); len(runs) != 1 || runs[0].(map[string]any)["phaseLabel"] != "Done (up)" || !reflect.DeepEqual(list["servedHostApis"], []any{float64(1)}) {
		t.Fatalf("list %v", list)
	}
	detail := shape("/api/runs/"+id, reflect.TypeFor[RunDetail]())
	if detail["verdictLabel"] != "Passed" || len(detail["services"].([]any)) != 1 || detail["services"].([]any)[0].(map[string]any)["open"] != "browser" || !slices.Contains(detail["allowedEvents"].([]any), any("stop")) {
		t.Fatalf("detail %v", detail)
	}
	if rep := shape("/api/runs/"+id+"/report", reflect.TypeFor[ReportView]()); rep["status"] != ReportMetaOnly {
		t.Fatalf("report %v", rep)
	}
	writeBundle(t, f.runDir(id))
	rep := shape("/api/runs/"+id+"/report", reflect.TypeFor[ReportView]())
	if rep["status"] != ReportBundle || rep["stoppedAt"].(map[string]any)["step"] != "a2" || len(rep["notProven"].(map[string]any)["skipped"].([]any)) != 1 {
		t.Fatalf("bundle report %v", rep)
	}
	kit := shape("/api/runs/"+id+"/kit", reflect.TypeFor[KitView]())
	if kit["exec"] == nil || !strings.HasPrefix(kit["exec"].(map[string]any)["command"].(string), "cockpit runs exec "+id) {
		t.Fatalf("kit %v", kit)
	}
	shape("/api/suites", reflect.TypeFor[SuitesView]())
	code := 0
	f.world.Exec = ExecView{ExitCode: &code, Flows: []ExecFlow{{ID: "alpha", Steps: []ExecStep{{ID: "a1", Status: contract.StepStatusPassed}}}}, Lines: []string{}}
	status, raw = f.post("/api/runs/"+id+"/exec", map[string]any{"flow": "alpha"})
	if status != http.StatusOK {
		t.Fatalf("exec %d %s", status, raw)
	}
	validate(t, reflect.TypeFor[ExecView](), raw)
	status, raw = f.post("/api/runs/"+id+"/keep", nil)
	if status != http.StatusOK || validate(t, reflect.TypeFor[OpReply](), raw)["run"].(map[string]any)["keep"] != true {
		t.Fatalf("keep %d %s", status, raw)
	}
	if checked != 6 {
		t.Fatalf("checked %d GET routes", checked)
	}
}

func TestRunItemKeepsEveryFieldThePreviousHubSentExceptTheRemovedHarnessFlag(t *testing.T) {
	have := properties(t, reflect.TypeFor[RunItem]())
	for _, field := range compatItemFields {
		if !slices.Contains(have, field) {
			t.Errorf("RunItem lost %s", field)
		}
	}
	for _, added := range []string{"hostApi", "cockpitVersion"} {
		if !slices.Contains(have, added) {
			t.Errorf("RunItem has no %s", added)
		}
	}
	detail := properties(t, reflect.TypeFor[RunDetail]())
	for _, field := range []string{"imageKeys", "imageDigests", "tail", "allowedEvents", "services", "runnerImage"} {
		if !slices.Contains(detail, field) {
			t.Errorf("RunDetail has no %s", field)
		}
	}
	if len(compatItemFields) != 24 {
		t.Fatal("field count")
	}
}

func TestReportViewSaysPendingForActiveRunsAndRedactionFailedWithTheFiles(t *testing.T) {
	f := newFixture(t, nil)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}, doneUp))
	f.seed(runMeta(2, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictError}, doneUp, func(m *store.Meta) {
		m.Redaction = &store.Redaction{Status: report.RedactionFailed, Files: []string{"signals/stream.jsonl"}}
	}))
	_, raw := f.get("/api/runs/" + rid(1) + "/report")
	if doc := validate(t, reflect.TypeFor[ReportView](), raw); doc["status"] != ReportPending {
		t.Fatalf("active %v", doc)
	}
	_, raw = f.get("/api/runs/" + rid(2) + "/report")
	doc := validate(t, reflect.TypeFor[ReportView](), raw)
	if doc["status"] != ReportRedactionFailed || doc["files"].([]any)[0] != "signals/stream.jsonl" {
		t.Fatalf("redaction %v", doc)
	}
}

func TestFailuresCarryAStableCodeAndTheStatusTheCLIMapsToExitCodes(t *testing.T) {
	f := newFixture(t, nil)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Queued, Env: lifecycle.EnvAbsent}))
	cases := []struct {
		method, path string
		body         any
		status       int
		code         string
	}{
		{http.MethodGet, "/api/runs/not-a-run", nil, 400, CodeBadRequest},
		{http.MethodGet, "/api/runs/" + rid(9), nil, 404, CodeNotFound},
		{http.MethodPost, "/api/runs/" + rid(1) + "/stop", map[string]any{"submitter": "cli"}, 409, "CONFLICT_STATE"},
		{http.MethodPost, "/api/runs/" + rid(1) + "/stop", map[string]any{}, 409, "CONFLICT_OWNER"},
		{http.MethodPost, "/api/runs", map[string]any{"worktree": "/w", "commit": "bad"}, 400, "REJECTED_COMMIT"},
		{http.MethodPost, "/api/runs", map[string]any{"worktree": "/w", "commit": "c1", "suite": "Bad Name"}, 400, CodeBadRequest},
		{http.MethodPost, "/api/runs", map[string]any{"commit": "c1"}, 400, CodeBadRequest},
		{http.MethodPost, "/api/runs/" + rid(1) + "/exec", map[string]any{}, 400, CodeBadRequest},
		{http.MethodGet, "/api/state", nil, 404, CodeNotFound},
		{http.MethodDelete, "/api/runs", nil, 404, CodeNotFound},
		{http.MethodGet, "/api/events?target=nope", nil, 400, CodeBadTarget},
	}
	for _, c := range cases {
		header := f.cli()
		status, raw := f.do(c.method, c.path, c.body, header)
		var p Problem
		if err := json.Unmarshal(raw, &p); err != nil || status != c.status || p.Code != c.code || p.Message == "" {
			t.Errorf("%s %s: %d %s, want %d %s", c.method, c.path, status, raw, c.status, c.code)
		}
		if !slices.Contains(Codes, p.Code) {
			t.Errorf("%s is not a stable code", p.Code)
		}
	}
	_, raw := f.post("/api/runs/"+rid(1)+"/stop", map[string]any{"submitter": "cli"})
	var p Problem
	json.Unmarshal(raw, &p)
	if p.Run == nil || p.Run.PhaseLabel != "Queued" {
		t.Fatalf("conflict without the current run: %s", raw)
	}
	if len(cases) != 11 {
		t.Fatal("case count")
	}
}

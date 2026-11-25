package screen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/wire/wiretest"
)

func recordedFlows(t *testing.T, f *fixture) string {
	t.Helper()
	doneWire(f, 1)
	dir := f.runDir(rid(1))
	manifest := contract.Manifest{SchemaVersion: contract.RunnerSchemaVersion, Suite: "toy",
		Stages: []contract.ManifestStage{{ID: "basic", Title: "Basic"}, {ID: "late", Title: "Late"}},
		Flows: []contract.ManifestFlow{
			{ID: "alpha", Title: "Alpha orders", Stage: "basic", Requires: []string{}, Steps: []contract.ManifestStep{{ID: "a1", Title: "Place an order", Kind: contract.StepKindRequest}, {ID: "a2", Title: "Pay the order", Kind: contract.StepKindRequest}}},
			{ID: "beta", Title: "Beta", Stage: "late", Requires: []string{}, Steps: []contract.ManifestStep{{ID: "b1", Title: "Quiet step", Kind: contract.StepKindRequest}}},
		}}
	result := wiretest.Result()
	result.SchemaVersion, result.Suite, result.Complete = contract.RunnerSchemaVersion, "toy", true
	writeJSON(t, filepath.Join(dir, api.RunnerDir, "manifest.json"), manifest)
	writeJSON(t, filepath.Join(dir, api.RunnerDir, "result.json"), result)
	wiretest.Seed(t, dir, planted)
	return "/runs/" + rid(1)
}

func TestTheCallsLinesMatchTheDocumentBothWays(t *testing.T) {
	var doc []string
	for _, row := range docTable(t, "| Why | Line |") {
		if m := ticked.FindStringSubmatch(row[1]); m != nil {
			doc = append(doc, m[1])
		}
	}
	if len(doc) != 10 || len(callsLines) != 10 {
		t.Fatalf("the document lists %d lines, the screen %d", len(doc), len(callsLines))
	}
	for _, line := range doc {
		if !slices.Contains(callsLines, line) {
			t.Errorf("the screen never writes %q", line)
		}
	}
	for _, line := range callsLines {
		if !slices.Contains(doc, line) {
			t.Errorf("the document does not list %q", line)
		}
	}
}

func TestTheCallsTabCountsTheCallsOfWhatRanAndSaysWhyItIsNotALinkOtherwise(t *testing.T) {
	f := newFixture(t)
	base := recordedFlows(t, f)
	cases := []struct{ path, want, absent string }{
		{base + "/report/alpha?step=a1", `/wire?flow=alpha&amp;step=a1" class="subtab" title="What each server was asked, and what it answered">Calls <span class="subtab-count">3</span>`, "data-calls-why"},
		{base + "/report/alpha", `/wire?flow=alpha&amp;scope=flow" class="subtab" title="What each server was asked, and what it answered">Calls <span class="subtab-count">7</span>`, "data-calls-why"},
		{base + "/report/beta?step=b1", `aria-disabled="true" title="` + callsStepSilent + `">Calls<`, "/wire?flow=beta"},
		{base + "/report/beta", `aria-disabled="true" title="` + callsFlowSilent + `">Calls<`, "/wire?flow=beta"},
	}
	for _, c := range cases {
		_, _, body := f.get(c.path)
		region := section(t, body, "flow")
		if !strings.Contains(region, c.want) || strings.Contains(region, c.absent) {
			t.Errorf("%s: lacks %q or holds %q", c.path, c.want, c.absent)
		}
	}
	if _, _, steps := f.get(base + "/report/alpha"); !strings.Contains(steps, `<span class="text-fg">3 calls</span>`) || !strings.Contains(steps, `<span class="text-fg">4 calls</span>`) {
		t.Fatal("the Steps pane does not count each step's calls")
	}
}

func TestACallsTabWithoutAStoredSuiteRecordSaysWhichStateTheRecordIsIn(t *testing.T) {
	f := newFixture(t)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictFailed}, doneUp, wireRun))
	dir := f.runDir(rid(1))
	writeRunner(t, dir, "boom")
	writeBundle(t, dir)
	_, _, untaken := f.get("/runs/" + rid(1) + "/report/alpha?step=a1")
	if !strings.Contains(untaken, callsNotTaken) || strings.Contains(untaken, "/wire?") {
		t.Fatal("a run that ended without a suite record links to its calls, or does not say so")
	}
	var sum contract.Summary
	raw, err := os.ReadFile(filepath.Join(dir, report.DirReport, report.FileSummary))
	if err != nil || json.Unmarshal(raw, &sum) != nil {
		t.Fatal("the bundle summary does not read")
	}
	sum.Wire = &contract.WireStatus{State: contract.WireStateLost, Detail: "the recorder restarted"}
	writeJSON(t, filepath.Join(dir, report.DirReport, report.FileSummary), sum)
	_, _, lost := f.get("/runs/" + rid(1) + "/report/alpha?step=a1")
	if !strings.Contains(lost, "The record of the suite was not stored (lost).") || strings.Contains(lost, callsNotTaken) {
		t.Fatal("a lost suite record is not named on the Calls tab")
	}
}

func TestTheRunHeaderSaysWhetherTheRunRecordsItsCalls(t *testing.T) {
	f := newFixture(t)
	finished(f)
	doneWire(f, 2)
	_, _, plain := f.get("/runs/" + rid(1))
	if !strings.Contains(runHeadOf(t, plain), ">calls not recorded<") || !strings.Contains(plain, wireNoRecord) {
		t.Fatal("a run without wire does not say that it records no calls")
	}
	_, _, recorded := f.get("/runs/" + rid(2))
	head := runHeadOf(t, recorded)
	if !strings.Contains(head, `href="/runs/`+rid(2)+`/wire"`) || !strings.Contains(head, ">calls recorded<") || strings.Contains(recorded, wireNoRecord) {
		t.Fatal("a run with wire does not link its calls from the header")
	}
}

func TestThePickedCallNamesItsStepByIdAndTitleAndSaysWhetherItIsTiedToIt(t *testing.T) {
	f := newFixture(t)
	base := recordedFlows(t, f) + "/wire"
	cases := []struct {
		call string
		want []string
	}{
		{"1", []string{`call-step-lead">in<`, "alpha › a1", "Place an order", "Alpha orders", `href="/runs/` + rid(1) + `/report/alpha?step=a1"`}},
		{"3", []string{`call-step-lead">during<`, "alpha › a1", "Place an order"}},
		{"8", []string{`call-step-lead">in<`, "Outside any step"}},
	}
	for _, c := range cases {
		_, _, body := f.get(base + "?flow=alpha&scope=flow&call=" + c.call)
		panel := section(t, body, wireCallPanelID)
		for _, want := range c.want {
			if !strings.Contains(panel, want) {
				t.Errorf("call %s: the panel lacks %q", c.call, want)
			}
		}
	}
}

func TestAFlowSelectionLabelsEachStepOfTheTableAndTheSequenceByIdAndTitle(t *testing.T) {
	f := newFixture(t)
	base := recordedFlows(t, f) + "/wire?flow=alpha&scope=flow"
	_, _, table := f.get(base + "&view=calls")
	bands := strings.Count(section(t, table, "wire"), "data-band")
	for _, want := range []string{"step a1 · Place an order", "step a2 · Pay the order", "During this flow, not tied to its steps"} {
		if !strings.Contains(table, want) {
			t.Errorf("the table lacks %q", want)
		}
	}
	if bands != 4 {
		t.Fatalf("the table holds %d step groups", bands)
	}
	_, _, step := f.get(recordedFlows(t, newFixture(t)) + "/wire?flow=alpha&step=a1&view=calls")
	if strings.Contains(section(t, step, "wire"), "data-band") {
		t.Fatal("a step selection is cut into groups")
	}
	_, _, sequence := f.get(base)
	for _, want := range []string{">step a1 · Place an order <", ">step a2 · Pay the order <", ": step a1 · Place an order\n", ": step a2 · Pay the order\n"} {
		if !strings.Contains(sequence, want) {
			t.Errorf("the sequence lacks %q", want)
		}
	}
}

func TestABandLabelIsCutToTheWidthOfTheDiagram(t *testing.T) {
	long := strings.Repeat("가", 80)
	if got := wireFit("step a1 · "+long, 300); !strings.HasSuffix(got, "…") || len([]rune(got)) >= len([]rune(long)) {
		t.Fatalf("cut %q", got)
	}
	if got := wireFit("step a1 · short", 300); got != "step a1 · short" {
		t.Fatalf("canary: a short label stays whole: %q", got)
	}
}

func runHeadOf(t *testing.T, html string) string {
	t.Helper()
	start := strings.Index(html, `id="run-head"`)
	if start < 0 {
		t.Fatal("no run header")
	}
	return html[start : start+strings.Index(html[start:], "</header>")]
}

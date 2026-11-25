package screen

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/wire/wiretest"
)

func treeFixture() []contract.FlowSummary {
	took := int64(40)
	return []contract.FlowSummary{
		{ID: "alpha", Stage: "basic", Status: contract.FlowOutcomePassed, Steps: 2, Passed: 2, StepList: []contract.FlowStep{
			{ID: "a1", Title: "A1", Status: contract.StepStatusPassed, DurationMs: &took},
			{ID: "a2", Title: "A2", Status: contract.StepStatusPassed},
		}},
		{ID: "beta", Stage: "basic", Status: contract.FlowOutcomeFailed, Steps: 3, Passed: 1, Failed: 1, Skipped: 1, StepList: []contract.FlowStep{
			{ID: "b1", Title: "B1", Status: contract.StepStatusPassed},
			{ID: "b2", Title: "B2", Status: contract.StepStatusFailed},
			{ID: "b3", Title: "B3", Status: contract.StepStatusSkipped},
		}},
		{ID: "gamma", Stage: "late", Status: contract.FlowOutcomeSkipped, Steps: 1, Skipped: 1, StepList: []contract.FlowStep{
			{ID: "g1", Title: "G1", Status: contract.StepStatusSkipped},
		}},
	}
}

func plainLinks() treeLinks {
	return treeLinks{
		flow:   func(flow string) string { return "/" + flow },
		step:   func(flow, step string) string { return "/" + flow + "/" + step },
		filter: func(only string) string { return "?only=" + only },
		meta:   stepTook,
	}
}

func treeShape(t flowTree) string {
	var out []string
	for _, stage := range t.stages {
		for _, f := range stage.flows {
			row := stage.name + ":" + f.id
			for _, s := range f.steps {
				row += "+" + s.id
			}
			out = append(out, row)
		}
	}
	return strings.Join(out, " ")
}

func TestTheFlowTreeGroupsFlowsByStageAndListsTheStepsOfThePickedFlowOnly(t *testing.T) {
	none := buildTree(treeFixture(), "", treePick{}, plainLinks())
	if got := treeShape(none); got != "basic:alpha basic:beta late:gamma" || len(none.stages) != 2 || none.rows != 3 {
		t.Fatalf("nothing picked: %q in %d stages, %d rows", got, len(none.stages), none.rows)
	}
	picked := buildTree(treeFixture(), "", treePick{flow: "beta", step: "b2"}, plainLinks())
	if got := treeShape(picked); got != "basic:alpha basic:beta+b1+b2+b3 late:gamma" || picked.rows != 6 {
		t.Fatalf("a picked step: %q, %d rows", got, picked.rows)
	}
	beta := picked.stages[0].flows[1]
	if beta.on || !beta.steps[1].on || beta.steps[0].on || beta.count != "1 / 3" || beta.steps[1].href != "/beta/b2" {
		t.Fatalf("the picked step is not the one marked: %+v", beta)
	}
	flow := buildTree(treeFixture(), "", treePick{flow: "alpha"}, plainLinks())
	alpha := flow.stages[0].flows[0]
	if !alpha.on || alpha.steps[0].on || alpha.steps[0].meta != "40ms" || alpha.steps[1].meta != "" {
		t.Fatalf("a picked flow: %+v", alpha)
	}
}

func TestOnlyFailedAndOnlySkippedLeaveTheFlowsThatHoldSuchAStepWithThoseSteps(t *testing.T) {
	cases := map[string]string{
		onlyFailed:  "basic:beta+b2",
		onlySkipped: "basic:beta+b3 late:gamma+g1",
		"":          "basic:alpha basic:beta late:gamma",
	}
	for only, want := range cases {
		tree := buildTree(treeFixture(), only, treePick{}, plainLinks())
		if got := treeShape(tree); got != want {
			t.Errorf("only=%q lists %q, want %q", only, got, want)
		}
		counts := []int{tree.filters[0].count, tree.filters[1].count, tree.filters[2].count}
		if counts[0] != 6 || counts[1] != 1 || counts[2] != 2 {
			t.Errorf("only=%q counts %v", only, counts)
		}
		for _, f := range tree.filters {
			if f.on != (f.href == "?only="+only) {
				t.Errorf("only=%q marks the filter %s wrongly", only, f.label)
			}
		}
	}
	if len(cases) != 3 {
		t.Fatal("case count")
	}
}

func liveFlows(t *testing.T, f *fixture) string {
	t.Helper()
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}, doneUp))
	dir := f.runDir(rid(1))
	manifest := contract.Manifest{SchemaVersion: contract.RunnerSchemaVersion, Suite: "toy",
		Stages: []contract.ManifestStage{{ID: "basic", Title: "Basic"}, {ID: "late", Title: "Late"}},
		Flows: []contract.ManifestFlow{
			{ID: "alpha", Title: "Alpha", Stage: "basic", Requires: []string{}, Steps: []contract.ManifestStep{{ID: "a1", Title: "A1", Kind: contract.StepKindRequest}}},
			{ID: "beta", Title: "Beta", Stage: "late", Requires: []string{"alpha"}, Steps: []contract.ManifestStep{{ID: "b1", Title: "B1", Kind: contract.StepKindRequest}, {ID: "b2", Title: "B2", Kind: contract.StepKindWait}}},
		}}
	result := contract.Result{SchemaVersion: contract.RunnerSchemaVersion, Suite: "toy", Complete: true, StartedAt: "2026-10-19T01:00:00.000Z",
		Steps: []contract.ResultStep{
			{Flow: "alpha", ID: "a1", Status: contract.StepStatusPassed, Requests: 1},
			{Flow: "beta", ID: "b1", Status: contract.StepStatusFailed, Requests: 1, Message: "status 500"},
			{Flow: "beta", ID: "b2", Status: contract.StepStatusSkipped},
		}}
	writeJSON(t, filepath.Join(dir, api.RunnerDir, "manifest.json"), manifest)
	writeJSON(t, filepath.Join(dir, api.RunnerDir, "result.json"), result)
	return "/runs/" + rid(1) + "/report"
}

func TestTheReportAndFlowScreensShareOneTreeThatMarksThePickAndKeepsItsFilter(t *testing.T) {
	f := newFixture(t)
	base := liveFlows(t, f)
	_, _, report := f.get(base)
	tree := section(t, report, "report")
	for _, want := range []string{`aria-label="Flows"`, ">basic<", ">late<", `href="` + base + `/alpha"`, `href="` + base + `/beta"`, `href="` + base + `?only=failed"`, ">1 / 1<", ">0 / 2<"} {
		if !strings.Contains(tree, want) {
			t.Errorf("the report tree lacks %q", want)
		}
	}
	if strings.Contains(tree, `aria-current="true"`) || strings.Contains(tree, base+`/beta?step=b1"`) {
		t.Fatal("with nothing picked the tree marks a row or lists steps")
	}
	_, _, step := f.get(base + "/beta?step=b1&only=failed")
	flow := section(t, step, "flow")
	for _, want := range []string{
		`href="` + base + `/beta?only=failed&amp;step=b1" class="tree-step tree-on"`, `data-scroll-here="tree-beta/b1"`,
		`href="` + base + `/beta?only=failed&amp;pane=source&amp;step=b1"`, `aria-current="page">Step`, "status 500",
	} {
		if !strings.Contains(flow, want) {
			t.Errorf("the flow screen lacks %q", want)
		}
	}
	if strings.Contains(flow, `/alpha`) || strings.Contains(flow, "step=b2") {
		t.Fatal("only=failed still lists a flow or a step that did not fail")
	}
	_, _, whole := f.get(base + "/beta")
	if list := section(t, whole, "flow"); !strings.Contains(list, `aria-current="page">Steps`) || !strings.Contains(list, ">b2<") || !strings.Contains(list, "requires alpha") {
		t.Fatal("a picked flow does not list its steps and its facts")
	}
}

func TestSkippedStepsAreGroupedByFlowInTheOrderTheReportListsThem(t *testing.T) {
	np := &contract.NotProven{Skipped: []string{"alpha › a3", "alpha › a4", "beta › b1", "odd"}, Declared: []string{"gamma"}, Unread: []string{"wire"}}
	groups := skippedByFlow(np)
	if len(groups) != 3 || groups[0].flow != "alpha" || strings.Join(groups[0].steps, ",") != "a3,a4" || groups[1].flow != "beta" || groups[2].flow != "odd" || len(groups[2].steps) != 0 {
		t.Fatalf("%+v", groups)
	}
	if skippedCount(np) != 4 || skippedCount(nil) != 0 || skippedByFlow(nil) != nil {
		t.Fatal("counts")
	}
	items := notProvenItems(np)
	if len(items) != 2 || !strings.HasPrefix(items[0], "declared by the suite · gamma") || items[1] != "unread record · wire" {
		t.Fatalf("%v", items)
	}
}

func TestTheCallTreeOfAnExecTakeListsThatTakesFlowsWithItsOwnOutcomes(t *testing.T) {
	f := newFixture(t)
	doneWire(f, 1)
	dir := f.runDir(rid(1))
	manifest := contract.Manifest{SchemaVersion: contract.RunnerSchemaVersion, Suite: "toy",
		Stages: []contract.ManifestStage{{ID: "basic", Title: "Basic"}, {ID: "late", Title: "Late"}},
		Flows: []contract.ManifestFlow{
			{ID: "alpha", Title: "Alpha", Stage: "basic", Requires: []string{}, Steps: []contract.ManifestStep{{ID: "a1", Title: "A1", Kind: contract.StepKindRequest}, {ID: "a2", Title: "A2", Kind: contract.StepKindRequest}}},
			{ID: "beta", Title: "Beta", Stage: "late", Requires: []string{}, Steps: []contract.ManifestStep{{ID: "b1", Title: "B1", Kind: contract.StepKindRequest}}},
		}}
	suite := wiretest.Result()
	suite.SchemaVersion, suite.Suite, suite.Complete = contract.RunnerSchemaVersion, "toy", true
	writeJSON(t, filepath.Join(dir, api.RunnerDir, "manifest.json"), manifest)
	writeJSON(t, filepath.Join(dir, api.RunnerDir, "result.json"), suite)
	wiretest.Seed(t, dir, planted)
	again := wiretest.T0.Add(time.Minute)
	wiretest.Store(t, dir, wiretest.Take{Name: "exec-1", Calls: wiretest.Calls("x")[:1], Routes: wiretest.Routes(), Result: &contract.Result{Steps: []contract.ResultStep{
		{Flow: "beta", ID: "b1", Status: contract.StepStatusFailed, StartedAt: again.Format(time.RFC3339Nano), FinishedAt: again.Add(time.Second).Format(time.RFC3339Nano)},
	}}})
	base := "/runs/" + rid(1) + "/wire"
	_, _, whole := f.get(base + "?flow=beta&step=b1")
	first := section(t, whole, "wire")
	for _, want := range []string{"flow=alpha&amp;scope=flow", ">basic<", ">late<", `aria-current="page">Calls <span class="subtab-count">0</span>`, `All <span class="tabular-nums">3<`, `Failed <span class="tabular-nums">1<`} {
		if !strings.Contains(first, want) {
			t.Errorf("canary: the suite take lacks %q", want)
		}
	}
	_, _, exec := f.get(base + "?take=exec-1&flow=beta&step=b1")
	region := section(t, exec, "wire")
	for _, want := range []string{"take exec-1", ">late<", `All <span class="tabular-nums">1<`, `Failed <span class="tabular-nums">1<`, `class="badge tone-bad">Failed<`, "0 of 1 steps passed in this take."} {
		if !strings.Contains(region, want) {
			t.Errorf("the exec take lacks %q", want)
		}
	}
	if strings.Contains(region, "flow=alpha") || strings.Contains(region, ">basic<") || strings.Contains(region, ">Source<") {
		t.Fatalf("the exec take offers a flow it did not run, or the panes of the suite's run: %s", region)
	}
	if _, _, gone := f.get(base + "?take=exec-1&flow=alpha&scope=flow"); !strings.Contains(gone, "The record holds no call here: this take has no such flow.") || strings.Contains(gone, "cannot be read") {
		t.Fatal("canary: the exec take holds no flow alpha, and says so without calling the record unreadable")
	}
}

func TestCallsAreOfferedOnlyForAFlowOrAStepThatRan(t *testing.T) {
	cases := []struct {
		flow, step string
		want       bool
	}{{"alpha", "a1", true}, {"beta", "b2", true}, {"beta", "b3", false}, {"beta", "", true}, {"gamma", "", false}, {"gamma", "g1", false}}
	for _, c := range cases {
		flow := findFlow(treeFixture(), c.flow)
		if got := ranIn(*flow, pickedStep(*flow, c.step)); got != c.want {
			t.Errorf("%s › %q: ran %v, want %v", c.flow, c.step, got, c.want)
		}
	}
	if len(cases) != 6 {
		t.Fatal("case count")
	}
	f := newFixture(t)
	base := liveFlows(t, f)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}, doneUp, wireRun))
	if _, _, ran := f.get(base + "/beta?step=b1"); !strings.Contains(ran, callsPending) || strings.Contains(ran, "/wire?") {
		t.Fatal("a step of a run still testing links to calls its record does not hold yet, or does not say why")
	}
	if _, _, skipped := f.get(base + "/beta?step=b2"); !strings.Contains(skipped, callsStepIdle) || strings.Contains(skipped, "/wire?") || !strings.Contains(skipped, ">Source<") {
		t.Fatal("a skipped step links to calls it cannot have, does not say why, or lost its Source pane")
	}
}

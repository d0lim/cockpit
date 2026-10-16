package runner

import (
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

func decl(flows ...FlowDecl) SuiteDecl {
	return SuiteDecl{
		Name: "fake",
		Stages: []contract.ManifestStage{
			{ID: "boot", Title: "Bootstrap"},
			{ID: "inbound", Title: "Inbound", NotProven: "no stand-in yet"},
			{ID: "use", Title: "Use"},
		},
		Flows: flows,
	}
}

func fd(id, stage string, once bool, requires []string, steps ...string) FlowDecl {
	f := FlowDecl{ID: id, Title: id + " flow", Stage: stage, Once: once, Requires: requires, Source: &contract.ManifestSource{File: "flows/fake.ts", Line: 7}}
	if f.Requires == nil {
		f.Requires = []string{}
	}
	for _, s := range steps {
		f.Steps = append(f.Steps, StepDecl{ID: s, Title: id + " " + s, Kind: contract.StepKindRequest, Source: &contract.ManifestSource{File: "flows/fake.ts", Line: 13}})
	}
	return f
}

var coreDecl = decl(
	fd("alpha", "boot", true, nil, "a1"),
	fd("beta", "use", false, []string{"alpha"}, "b1", "b2", "b3"),
	fd("gamma", "use", false, []string{"beta"}, "g1"),
)

func ledgerOf(entries map[string]contract.FlowStatus) contract.Ledger {
	l := contract.Ledger{SchemaVersion: contract.RunnerSchemaVersion, Flows: map[string]contract.LedgerEntry{}}
	for id, s := range entries {
		l.Flows[id] = contract.LedgerEntry{Status: s, At: ledgerAt}
	}
	return l
}

func ids(plans []ExecPlan) [][]string {
	out := [][]string{}
	for _, p := range plans {
		var names []string
		for _, f := range p.Flows {
			names = append(names, f.ID)
		}
		out = append(out, names)
	}
	return out
}

func TestManifestOrdersFlowsByStageAndCopiesStepKindOnceAndRequires(t *testing.T) {
	s := decl(fd("late", "use", false, nil, "u1"), fd("early", "boot", true, nil, "e1", "e2"))
	ordered, err := Order(s)
	if err != nil {
		t.Fatal(err)
	}
	m := Manifest(s, ordered, contract.RunnerIdentity{Version: "v", Image: "img"})
	if m.SchemaVersion != 2 || m.Suite != "fake" || m.Runner == nil || m.Runner.Image != "img" {
		t.Fatalf("%+v", m)
	}
	if len(m.Stages) != 3 || m.Stages[1].NotProven != "no stand-in yet" || m.Flows[0].ID != "early" || m.Flows[1].ID != "late" {
		t.Fatalf("%+v", m)
	}
	early := m.Flows[0]
	if early.Title != "early flow" || early.Stage != "boot" || !early.Once || len(early.Requires) != 0 || early.Source == nil || early.Source.File != "flows/fake.ts" {
		t.Fatalf("%+v", early)
	}
	if len(early.Steps) != 2 {
		t.Fatalf("%+v", early.Steps)
	}
	if s := early.Steps[1]; s.ID != "e2" || s.Title != "early e2" || s.Kind != "request" || s.Source == nil || *s.Source != (contract.ManifestSource{File: "flows/fake.ts", Line: 13}) {
		t.Fatalf("%+v", s)
	}
	bare := fd("bare", "use", false, nil, "b1")
	bare.Source = nil
	bare.Steps[0].Source = nil
	m = Manifest(decl(bare), []FlowDecl{bare}, contract.RunnerIdentity{})
	if m.Flows[0].Source != nil || m.Flows[0].Steps[0].Source != nil {
		t.Fatalf("%+v", m.Flows[0])
	}
}

func TestOrderRejectsUnknownStageUnknownOrLaterRequireDuplicateIDAndBadShape(t *testing.T) {
	bad := []struct {
		flows []FlowDecl
		word  string
	}{
		{[]FlowDecl{fd("x1", "nope", false, nil, "s")}, "nope"},
		{[]FlowDecl{fd("x1", "use", false, []string{"ghost"}, "s")}, "ghost"},
		{[]FlowDecl{fd("x1", "boot", false, []string{"x2"}, "s"), fd("x2", "use", false, nil, "s")}, "x2"},
		{[]FlowDecl{fd("x1", "use", false, nil, "s"), fd("x1", "use", false, nil, "t")}, "x1"},
		{[]FlowDecl{fd("Bad", "use", false, nil, "s")}, "Bad"},
		{[]FlowDecl{fd("x1", "use", false, nil, "s", "s")}, `"s"`},
		{[]FlowDecl{{ID: "x1", Title: "t", Stage: "use", Requires: []string{}, Steps: []StepDecl{{ID: "s", Title: "s", Kind: "fly"}}}}, "fly"},
	}
	for _, b := range bad {
		_, err := Order(decl(b.flows...))
		if err == nil || !strings.Contains(err.Error(), b.word) {
			t.Fatalf("%v: %v", b.word, err)
		}
	}
}

func TestAE4PlanRerunsRepeatableRequiresAndStopsAtPassedOnceRequire(t *testing.T) {
	flows, _ := Order(coreDecl)
	plans, err := PlanExec(flows, []string{"gamma"}, ledgerOf(map[string]contract.FlowStatus{"alpha": "passed", "beta": "passed", "gamma": "passed"}))
	if err != nil || !slices.Equal(ids(plans)[0], []string{"beta", "gamma"}) {
		t.Fatalf("%v %v", ids(plans), err)
	}
	plans, err = PlanExec(flows, []string{"beta"}, ledgerOf(map[string]contract.FlowStatus{"alpha": "passed"}))
	if err != nil || !slices.Equal(ids(plans)[0], []string{"beta"}) {
		t.Fatalf("%v %v", ids(plans), err)
	}
}

func TestAE4AE3PlanRejectsUnpassedOnceRequireAndRanOnceTargetButAllowsUnstartedOnce(t *testing.T) {
	flows, _ := Order(coreDecl)
	for _, l := range []contract.Ledger{ledgerOf(nil), ledgerOf(map[string]contract.FlowStatus{"alpha": "failed"}), ledgerOf(map[string]contract.FlowStatus{"alpha": "started"})} {
		if _, err := PlanExec(flows, []string{"gamma"}, l); err == nil || !strings.Contains(err.Error(), "alpha") {
			t.Fatalf("%v", err)
		}
	}
	for _, s := range []contract.FlowStatus{"started", "passed", "failed"} {
		if _, err := PlanExec(flows, []string{"alpha"}, ledgerOf(map[string]contract.FlowStatus{"alpha": s})); err == nil || !strings.Contains(err.Error(), "already ran") {
			t.Fatalf("%s: %v", s, err)
		}
	}
	for _, l := range []contract.Ledger{ledgerOf(nil), ledgerOf(map[string]contract.FlowStatus{"alpha": "skipped"})} {
		if plans, err := PlanExec(flows, []string{"alpha"}, l); err != nil || !slices.Equal(ids(plans)[0], []string{"alpha"}) {
			t.Fatalf("%v %v", ids(plans), err)
		}
	}
	if _, err := PlanExec(flows, []string{"ghost"}, ledgerOf(nil)); err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("%v", err)
	}
	plans, err := PlanExec(flows, []string{"alpha", "beta"}, ledgerOf(nil))
	if err != nil || len(plans) != 2 || !slices.Equal(ids(plans)[1], []string{"beta"}) {
		t.Fatalf("%v %v", ids(plans), err)
	}
	if _, err := PlanExec(flows, []string{"alpha", "alpha"}, ledgerOf(nil)); err == nil {
		t.Fatal("the same once target was accepted twice")
	}
}

func TestSummaryLineCarriesOnlyFlowStepStatusDurationAndHTTPStatus(t *testing.T) {
	ms := int64(1000)
	status := 201
	if got := SummaryLine(contract.ResultStep{Flow: "a", ID: "s", Status: "passed", DurationMs: &ms, HTTPStatus: &status, Message: "PLANTED"}); got != "a › s passed 1000ms [201]" {
		t.Fatalf("%q", got)
	}
	if got := SummaryLine(contract.ResultStep{Flow: "a", ID: "s", Status: "skipped"}); got != "a › s skipped" {
		t.Fatalf("%q", got)
	}
}

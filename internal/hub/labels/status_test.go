package labels

import (
	"reflect"
	"testing"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var statusLabelMutations = []porttest.Mutation{
	{"every flow status has a label", "drop the notProven entry from flowStatus", "red: the not proven label is empty", "TestEveryStepStatusAndFlowOutcomeHasAFlowStatusLabel"},
	{"every collector state has a label", "drop the report-only entry from collectors", "red: report-only has no label", "TestEveryCollectorStateHasALabel"},
}

func TestEveryStepStatusAndFlowOutcomeHasAFlowStatusLabel(t *testing.T) {
	table := Load()
	checked := 0
	for _, e := range contract.Enums() {
		if e.Type != reflect.TypeFor[contract.StepStatus]() && e.Type != reflect.TypeFor[contract.FlowOutcome]() {
			continue
		}
		for _, v := range e.Values {
			if table.FlowStatus[v.(string)] == "" {
				t.Errorf("%s %v has no flow status label", e.Type.Name(), v)
			}
			checked++
		}
	}
	if checked != 10 || table.FlowStatus[FlowNotProven] == "" || table.FlowStatus["unknown"] != "" {
		t.Fatalf("checked %d values, not proven %q", checked, table.FlowStatus[FlowNotProven])
	}
}

func TestEveryCollectorStateHasALabel(t *testing.T) {
	table := Load()
	states := []collect.State{collect.StateArmed, collect.StateBackoff, collect.StateStopped, collect.StateReportOnly}
	for _, s := range states {
		if table.Collectors[string(s)] == "" {
			t.Errorf("collector state %s has no label", s)
		}
	}
	if len(states) != 4 || len(table.Collectors) != len(states) || table.Collectors["report-only"] != "Report only" {
		t.Fatalf("collector labels %v", table.Collectors)
	}
}

func TestStatusLabelMutationRecordMatchesThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "status_test.go", statusLabelMutations)
}

package wire_test

import (
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/wire"
)

func TestTheRunnerServiceOfTheHarnessIsTheCallerLinkedByItsStepWindow(t *testing.T) {
	start := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	result := &contract.Result{Steps: []contract.ResultStep{{Flow: "f", ID: "s", StartedAt: start.Format(time.RFC3339Nano), FinishedAt: start.Add(time.Second).Format(time.RFC3339Nano)}}}
	calls := []wire.Call{{Seq: 1, Caller: compose.RoleRunner, Target: "orders", Started: start.Add(time.Millisecond), Finished: start.Add(2 * time.Millisecond)}}
	got := wire.LinkCalls(calls, result)
	if len(got) != 1 || got[0].Kind != wire.LinkExact || got[0].Step != 0 {
		t.Fatalf("links %+v", got)
	}
}

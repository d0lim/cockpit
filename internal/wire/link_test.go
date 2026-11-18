package wire

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
)

const (
	spanOne = "00f067aa0ba902b7"
	spanTwo = "11f067aa0ba902b8"
)

func at(ms int) time.Time { return callStart.Add(time.Duration(ms) * time.Millisecond) }

func iso(ms int) string { return at(ms).UTC().Format("2006-01-02T15:04:05.000Z") }

func linkResult() *contract.Result {
	return &contract.Result{Steps: []contract.ResultStep{
		{Flow: "f", ID: "first", StartedAt: iso(0), FinishedAt: iso(100), Calls: []contract.StepCall{{SpanID: spanOne}}},
		{Flow: "f", ID: "second", StartedAt: iso(200), FinishedAt: iso(300), Calls: []contract.StepCall{{SpanID: spanTwo}}},
		{Flow: "f", ID: "skipped"},
	}}
}

func call(seq int64, caller, target string, from, to int) Call {
	return Call{Seq: seq, Caller: caller, Target: target, Started: at(from), Finished: at(to)}
}

func withSpan(c Call, span string) Call {
	c.Span = span
	return c
}

func assertLinks(t *testing.T, got, want []Link) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("links\n got %+v\nwant %+v", got, want)
	}
}

func TestACallCarryingARecordedSpanIsExactWhereverItStarts(t *testing.T) {
	calls := []Call{
		withSpan(call(1, "orders", "ledger", 150, 160), spanTwo),
		withSpan(call(2, "cockpit-runner", "orders", 210, 220), spanOne),
	}
	assertLinks(t, LinkCalls(calls, linkResult()), []Link{{Kind: LinkExact, Step: 1}, {Kind: LinkExact, Step: 0}})
}

func TestRunnerCallsPastTheRecordedLimitAreExactByTheirStepWindow(t *testing.T) {
	var calls []Call
	var want []Link
	for i := range 60 {
		calls = append(calls, call(int64(i+1), runnerCaller, "orders", 10+i, 11+i))
		want = append(want, Link{Kind: LinkExact, Step: 0})
	}
	got := LinkCalls(calls, linkResult())
	assertLinks(t, got, want)
	if len(got) != 60 {
		t.Fatalf("linked %d calls", len(got))
	}
	other := LinkCalls([]Call{call(1, "orders", "ledger", 10, 11)}, linkResult())
	assertLinks(t, other, []Link{{Kind: LinkTime, Step: 0}})
}

func TestACallInsideACallToItsCallerIsNestedWithoutATraceparent(t *testing.T) {
	calls := []Call{
		call(1, runnerCaller, "orders", 10, 90),
		call(2, "orders", "ledger", 20, 60),
		call(3, "ledger", "audit", 30, 40),
	}
	assertLinks(t, LinkCalls(calls, linkResult()), []Link{
		{Kind: LinkExact, Step: 0},
		{Kind: LinkNested, Step: 0, Parent: 1},
		{Kind: LinkNested, Step: 0, Parent: 2},
	})
}

func TestTheTightestEnclosingCallIsTheParent(t *testing.T) {
	calls := []Call{
		call(1, runnerCaller, "orders", 5, 95),
		call(2, runnerCaller, "orders", 10, 30),
		call(3, runnerCaller, "orders", 12, 80),
		call(4, "orders", "ledger", 15, 20),
	}
	links := LinkCalls(calls, linkResult())
	if want := (Link{Kind: LinkNested, Step: 0, Parent: 2}); links[3] != want {
		t.Fatalf("got %+v, want %+v", links[3], want)
	}
}

func TestACallThatOutlivesItsCandidateParentIsNotNested(t *testing.T) {
	calls := []Call{
		call(1, runnerCaller, "orders", 10, 30),
		call(2, "orders", "ledger", 20, 40),
	}
	assertLinks(t, LinkCalls(calls, linkResult()), []Link{{Kind: LinkExact, Step: 0}, {Kind: LinkTime, Step: 0}})
}

func TestACallAnsweredBeforeItsParentEndedIsNestedEvenIfTheRecorderClosedItLater(t *testing.T) {
	child := call(2, "orders", "ledger", 20, 51)
	child.Replied = at(49)
	calls := []Call{call(1, "cockpit-runner", "orders", 10, 50), child}
	links := LinkCalls(calls, linkResult())
	if links[1].Kind != LinkNested || links[1].Parent != 1 {
		t.Fatalf("the answered child links %+v", links[1])
	}
	child.Replied = at(50)
	child.Finished = at(52)
	calls[1] = child
	if links = LinkCalls(calls, linkResult()); links[1].Kind != LinkNested {
		t.Fatalf("a child answered at the moment its parent ended links %+v", links[1])
	}
	child.Replied = at(51)
	calls[1] = child
	if links = LinkCalls(calls, linkResult()); links[1].Kind != LinkTime {
		t.Fatalf("a child answered after its parent ended links %+v", links[1])
	}
}

func TestATimeLinkNeverBecomesAParent(t *testing.T) {
	calls := []Call{
		call(1, "orders", "ledger", 10, 90),
		call(2, "ledger", "audit", 20, 30),
	}
	assertLinks(t, LinkCalls(calls, linkResult()), []Link{{Kind: LinkTime, Step: 0}, {Kind: LinkTime, Step: 0}})
}

func TestACallOutsideEveryStepHasNoLink(t *testing.T) {
	calls := []Call{
		call(1, "orders", "ledger", 150, 160),
		call(2, runnerCaller, "orders", 400, 410),
		call(3, "orders", "ledger", -50, -40),
	}
	none := Link{Kind: LinkNone, Step: NoStep}
	assertLinks(t, LinkCalls(calls, linkResult()), []Link{none, none, none})
	assertLinks(t, LinkCalls(calls[:1], nil), []Link{none})
}

func TestAStepWithoutAFinishTimeStaysOpenUntilTheTake(t *testing.T) {
	result := linkResult()
	result.Steps[1].FinishedAt = ""
	calls := []Call{
		call(1, runnerCaller, "orders", 60_000, 60_010),
		call(2, "orders", "ledger", 60_001, 60_002),
		call(3, "worker", "ledger", 90_000, 90_001),
	}
	assertLinks(t, LinkCalls(calls, result), []Link{
		{Kind: LinkExact, Step: 1},
		{Kind: LinkNested, Step: 1, Parent: 1},
		{Kind: LinkTime, Step: 1},
	})
}

func TestACallStillOpenAtTheTakeCanParentAndLinksCameBackInInputOrder(t *testing.T) {
	open := call(1, runnerCaller, "orders", 10, 0)
	open.Finished = time.Time{}
	calls := []Call{call(2, "orders", "ledger", 20, 30), open}
	assertLinks(t, LinkCalls(calls, linkResult()), []Link{{Kind: LinkNested, Step: 0, Parent: 1}, {Kind: LinkExact, Step: 0}})
}

func TestEveryLinkKindAppearsInOneRecord(t *testing.T) {
	calls := []Call{
		withSpan(call(1, "orders", "ledger", 150, 160), spanOne),
		call(2, "ledger", "audit", 152, 155),
		call(3, "worker", "ledger", 210, 211),
		call(4, "worker", "ledger", 500, 501),
	}
	seen := map[LinkKind]int{}
	for _, l := range LinkCalls(calls, linkResult()) {
		seen[l.Kind]++
	}
	if fmt.Sprint(seen) != "map[exact:1 nested:1 none:1 time:1]" {
		t.Fatalf("kinds %v", seen)
	}
}

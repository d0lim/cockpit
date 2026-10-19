package lifecycle

import (
	"encoding/json"
	"slices"
	"testing"
)

func mustNext(t *testing.T, s State, e Event, d Detail) State {
	t.Helper()
	got, err := Transition(&s, e, d)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestReportOfAnAbortedRunClosesAbortedEvenWhenTheSuiteVerdictIsPassed(t *testing.T) {
	got := mustNext(t, st(Reporting, EnvUp, VerdictAborted), Reported, Detail{Verdict: VerdictPassed})
	if got != st(Done, EnvUp, VerdictAborted) {
		t.Fatalf("%+v", got)
	}
}

func TestAbortedRunStaysAbortedWhenTheHubDiesDuringTheReport(t *testing.T) {
	got := mustNext(t, st(Reporting, EnvUp, VerdictAborted), Observed, up)
	if got != st(Reporting, EnvUp, VerdictAborted) {
		t.Fatalf("%+v", got)
	}
}

func TestReportedWithoutAVerdictClosesAsErrorInsteadOfPassing(t *testing.T) {
	got := mustNext(t, st(Reporting, EnvUp, VerdictNone), Reported, none)
	if got != st(Done, EnvUp, VerdictError) {
		t.Fatalf("%+v", got)
	}
}

func TestFailKeepsOnlyFailedAndClosesEverythingElseAsError(t *testing.T) {
	if got := mustNext(t, st(Building, EnvAbsent, VerdictNone), Fail, Detail{Verdict: VerdictFailed}); got.Verdict != VerdictFailed {
		t.Fatalf("%+v", got)
	}
	if got := mustNext(t, st(Building, EnvAbsent, VerdictNone), Fail, Detail{Verdict: VerdictPassed}); got.Verdict != VerdictError {
		t.Fatalf("%+v", got)
	}
}

func TestLostMarkStaysWhenReconciliationLooksAgain(t *testing.T) {
	lost := st(Done, EnvAbsent, VerdictPassed, Lost)
	if _, err := Transition(&lost, Observed, up); err == nil {
		t.Fatal("lost run accepted an observation")
	}
	if _, err := Transition(&lost, Destroy, none); err == nil {
		t.Fatal("lost run accepted destroy")
	}
	alive := st(Done, EnvAbsent, VerdictPassed)
	if alive.Key() != "done/absent" || lost.Key() != "lost" {
		t.Fatalf("canary: keys %s %s", alive.Key(), lost.Key())
	}
}

func TestAllowedEventsListTheEventsThatHaveACellInTheState(t *testing.T) {
	cases := []struct {
		key  Key
		want []Event
	}{
		{"testing", []Event{Tested, Destroy, Fail, Abort, Observed}},
		{"done/stopped", []Event{Resume, Destroy, Observed}},
		{"stopping", []Event{Stopped}},
		{KeyNew, []Event{Submit}},
		{"lost", nil},
	}
	for _, c := range cases {
		if got := AllowedEvents(samples[c.key], true); !slices.Equal(got, c.want) {
			t.Fatalf("%s: got %v, want %v", c.key, got, c.want)
		}
	}
	if len(cases) != 5 {
		t.Fatal("case count")
	}
}

func TestStateLabelsCoverEveryStateKey(t *testing.T) {
	cases := []struct {
		s    State
		want string
	}{
		{st(Queued, EnvAbsent, VerdictNone), "Queued"},
		{st(Preparing, EnvAbsent, VerdictNone), "Preparing"},
		{st(Building, EnvAbsent, VerdictNone), "Building"},
		{st(Sealing, EnvAbsent, VerdictNone), "Checking the seal"},
		{st(Booting, EnvAbsent, VerdictNone), "Booting"},
		{st(Testing, EnvUp, VerdictNone), "Running"},
		{st(Reporting, EnvUp, VerdictNone), "Reporting"},
		{st(Done, EnvUp, VerdictNone), "Done (up)"},
		{st(Done, EnvStopped, VerdictNone), "Stopped"},
		{st(Done, EnvAbsent, VerdictNone), "Closed (no environment)"},
		{st(Done, EnvDestroyed, VerdictNone), "Destroyed"},
		{st(Stopping, EnvUp, VerdictNone), "Stopping"},
		{st(Resuming, EnvStopped, VerdictNone), "Resuming"},
		{st(Destroying, EnvUp, VerdictNone), "Destroying"},
		{st(Done, EnvAbsent, VerdictNone, Lost), "Lost"},
		{st(Done, EnvUp, VerdictNone, Orphan), "Orphan"},
	}
	for _, c := range cases {
		if got := Label(c.s); got != c.want {
			t.Fatalf("%+v: got %q, want %q", c.s, got, c.want)
		}
	}
	if len(cases) != len(stateLabels) {
		t.Fatalf("cases %d, labels %d", len(cases), len(stateLabels))
	}
}

func TestVerdictIsNullInJSONUntilItIsDecided(t *testing.T) {
	undecided, _ := json.Marshal(st(Queued, EnvAbsent, VerdictNone))
	decided, _ := json.Marshal(st(Done, EnvUp, VerdictPassed))
	if string(undecided) != `{"phase":"queued","env":"absent","verdict":null}` || string(decided) != `{"phase":"done","env":"up","verdict":"passed"}` {
		t.Fatalf("%s %s", undecided, decided)
	}
	var back State
	if err := json.Unmarshal(undecided, &back); err != nil || back.Verdict != VerdictNone {
		t.Fatalf("%+v %v", back, err)
	}
}

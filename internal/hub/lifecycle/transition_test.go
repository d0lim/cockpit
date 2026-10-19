package lifecycle

import (
	"errors"
	"slices"
	"sort"
	"testing"
)

func st(p Phase, e Env, v Verdict, m ...Mark) State {
	s := State{Phase: p, Env: e, Verdict: v}
	if len(m) > 0 {
		s.Mark = m[0]
	}
	return s
}

var samples = map[Key]*State{
	KeyNew:           nil,
	"queued":         new(st(Queued, EnvAbsent, VerdictNone)),
	"preparing":      new(st(Preparing, EnvAbsent, VerdictNone)),
	"building":       new(st(Building, EnvAbsent, VerdictNone)),
	"sealing":        new(st(Sealing, EnvAbsent, VerdictNone)),
	"booting":        new(st(Booting, EnvAbsent, VerdictNone)),
	"testing":        new(st(Testing, EnvUp, VerdictNone)),
	"reporting":      new(st(Reporting, EnvUp, VerdictNone)),
	"stopping":       new(st(Stopping, EnvUp, VerdictPassed)),
	"resuming":       new(st(Resuming, EnvStopped, VerdictPassed)),
	"destroying":     new(st(Destroying, EnvUp, VerdictPassed)),
	"done/up":        new(st(Done, EnvUp, VerdictPassed)),
	"done/stopped":   new(st(Done, EnvStopped, VerdictFailed)),
	"done/absent":    new(st(Done, EnvAbsent, VerdictError)),
	"done/destroyed": new(st(Done, EnvDestroyed, VerdictPassed)),
	"lost":           new(st(Done, EnvAbsent, VerdictPassed, Lost)),
	"orphan":         new(st(Done, EnvUp, VerdictNone, Orphan)),
}

type cell struct {
	key   Key
	event Event
	d     Detail
	next  State
}

var (
	up      = Detail{Observation: ObservedUp}
	stopped = Detail{Observation: ObservedStopped}
	gone    = Detail{Observation: ObservedGone}
	none    = Detail{}
)

var cells = []cell{
	{KeyNew, Submit, none, st(Queued, EnvAbsent, VerdictNone)},

	{"queued", TickStart, none, st(Preparing, EnvAbsent, VerdictNone)},
	{"queued", Destroy, none, st(Done, EnvDestroyed, VerdictAborted)},
	{"queued", Reject, none, st(Done, EnvAbsent, VerdictError)},

	{"preparing", Prepared, none, st(Building, EnvAbsent, VerdictNone)},
	{"preparing", Fail, none, st(Done, EnvAbsent, VerdictError)},
	{"preparing", Abort, none, st(Done, EnvAbsent, VerdictAborted)},
	{"preparing", Destroy, none, st(Destroying, EnvAbsent, VerdictAborted)},
	{"preparing", Observed, up, st(Done, EnvAbsent, VerdictError)},
	{"preparing", Observed, stopped, st(Done, EnvAbsent, VerdictError)},
	{"preparing", Observed, gone, st(Done, EnvAbsent, VerdictError)},

	{"building", Built, none, st(Sealing, EnvAbsent, VerdictNone)},
	{"building", Fail, none, st(Done, EnvAbsent, VerdictError)},
	{"building", Abort, none, st(Done, EnvAbsent, VerdictAborted)},
	{"building", Destroy, none, st(Destroying, EnvAbsent, VerdictAborted)},
	{"building", Observed, up, st(Done, EnvAbsent, VerdictError)},
	{"building", Observed, stopped, st(Done, EnvAbsent, VerdictError)},
	{"building", Observed, gone, st(Done, EnvAbsent, VerdictError)},

	{"sealing", Sealed, none, st(Booting, EnvAbsent, VerdictNone)},
	{"sealing", Fail, Detail{Verdict: VerdictFailed}, st(Done, EnvAbsent, VerdictFailed)},
	{"sealing", Abort, none, st(Done, EnvAbsent, VerdictAborted)},
	{"sealing", Destroy, none, st(Destroying, EnvAbsent, VerdictAborted)},
	{"sealing", Observed, up, st(Done, EnvAbsent, VerdictError)},
	{"sealing", Observed, stopped, st(Done, EnvAbsent, VerdictError)},
	{"sealing", Observed, gone, st(Done, EnvAbsent, VerdictError)},

	{"booting", Booted, none, st(Testing, EnvUp, VerdictNone)},
	{"booting", Fail, none, st(Done, EnvStopped, VerdictError)},
	{"booting", Abort, none, st(Done, EnvStopped, VerdictAborted)},
	{"booting", Destroy, none, st(Destroying, EnvAbsent, VerdictAborted)},
	{"booting", Observed, up, st(Done, EnvStopped, VerdictError)},
	{"booting", Observed, stopped, st(Done, EnvStopped, VerdictError)},
	{"booting", Observed, gone, st(Done, EnvAbsent, VerdictError)},

	{"testing", Tested, none, st(Reporting, EnvUp, VerdictNone)},
	{"testing", Fail, none, st(Reporting, EnvUp, VerdictError)},
	{"testing", Abort, none, st(Reporting, EnvUp, VerdictAborted)},
	{"testing", Destroy, none, st(Destroying, EnvUp, VerdictAborted)},
	{"testing", Observed, up, st(Reporting, EnvUp, VerdictError)},
	{"testing", Observed, stopped, st(Reporting, EnvStopped, VerdictError)},
	{"testing", Observed, gone, st(Reporting, EnvAbsent, VerdictError)},

	{"reporting", Reported, Detail{Verdict: VerdictPassed}, st(Done, EnvUp, VerdictPassed)},
	{"reporting", Fail, none, st(Done, EnvUp, VerdictError)},
	{"reporting", Abort, none, st(Reporting, EnvUp, VerdictAborted)},
	{"reporting", Destroy, none, st(Destroying, EnvUp, VerdictAborted)},
	{"reporting", Observed, up, st(Reporting, EnvUp, VerdictError)},
	{"reporting", Observed, stopped, st(Reporting, EnvStopped, VerdictError)},
	{"reporting", Observed, gone, st(Reporting, EnvAbsent, VerdictError)},

	{"stopping", Stopped, none, st(Done, EnvStopped, VerdictPassed)},

	{"resuming", Resumed, none, st(Done, EnvUp, VerdictPassed)},
	{"resuming", ResumeFailed, none, st(Done, EnvStopped, VerdictPassed)},

	{"destroying", Destroyed, none, st(Done, EnvDestroyed, VerdictPassed)},

	{"done/up", Stop, none, st(Stopping, EnvUp, VerdictPassed)},
	{"done/up", Destroy, none, st(Destroying, EnvUp, VerdictPassed)},
	{"done/up", Observed, up, st(Done, EnvUp, VerdictPassed)},
	{"done/up", Observed, stopped, st(Done, EnvStopped, VerdictPassed)},
	{"done/up", Observed, gone, st(Done, EnvAbsent, VerdictPassed, Lost)},

	{"done/stopped", Resume, none, st(Resuming, EnvStopped, VerdictFailed)},
	{"done/stopped", Destroy, none, st(Destroying, EnvStopped, VerdictFailed)},
	{"done/stopped", Observed, up, st(Done, EnvUp, VerdictFailed)},
	{"done/stopped", Observed, stopped, st(Done, EnvStopped, VerdictFailed)},
	{"done/stopped", Observed, gone, st(Done, EnvAbsent, VerdictFailed, Lost)},
}

func cellID(key Key, event Event, d Detail) string {
	if event == Observed {
		return string(key) + " observed:" + string(d.Observation)
	}
	return string(key) + " " + string(event)
}

func TestTransitionTableHasNineteenEventsAndSeventeenStateKeys(t *testing.T) {
	if len(Events) != 19 {
		t.Fatalf("events %d", len(Events))
	}
	var keys, sampled []string
	for k := range Transitions {
		keys = append(keys, string(k))
	}
	for k := range samples {
		sampled = append(sampled, string(k))
	}
	sort.Strings(keys)
	sort.Strings(sampled)
	if len(keys) != 17 || !slices.Equal(keys, sampled) {
		t.Fatalf("table keys %v, sample keys %v", keys, sampled)
	}
}

func TestCellsInTheTableEqualTheCellsListedInThisFile(t *testing.T) {
	var inTable, listed []string
	for key, row := range Transitions {
		for e := range row.On {
			inTable = append(inTable, cellID(key, e, none))
		}
		for o := range row.Observed {
			inTable = append(inTable, cellID(key, Observed, Detail{Observation: o}))
		}
	}
	for _, c := range cells {
		listed = append(listed, cellID(c.key, c.event, c.d))
	}
	sort.Strings(inTable)
	sort.Strings(listed)
	if len(listed) != 60 || !slices.Equal(inTable, listed) {
		t.Fatalf("table %v\nlisted %v", inTable, listed)
	}
}

func TestEveryListedCellTransitionsToItsNextState(t *testing.T) {
	ran := 0
	for _, c := range cells {
		t.Run(cellID(c.key, c.event, c.d), func(t *testing.T) {
			ran++
			got, err := Transition(samples[c.key], c.event, c.d)
			if err != nil || got != c.next {
				t.Fatalf("got %+v err %v, want %+v", got, err, c.next)
			}
		})
	}
	if ran != len(cells) {
		t.Fatalf("ran %d of %d", ran, len(cells))
	}
}

func TestEveryCellNotInTheTableIsAConflict(t *testing.T) {
	listed := map[string]bool{}
	for _, c := range cells {
		listed[cellID(c.key, c.event, c.d)] = true
	}
	checked := 0
	var accepted []string
	for key, s := range samples {
		for _, e := range Events {
			details := []Detail{none}
			if e == Observed {
				details = []Detail{up, stopped, gone}
			}
			for _, d := range details {
				if listed[cellID(key, e, d)] {
					continue
				}
				checked++
				var conflict *Conflict
				if _, err := Transition(s, e, d); !errors.As(err, &conflict) {
					accepted = append(accepted, cellID(key, e, d))
				}
			}
		}
	}
	if len(accepted) != 0 {
		t.Fatalf("accepted %v", accepted)
	}
	if checked != 17*21-60 {
		t.Fatalf("checked %d", checked)
	}
}

func TestConflictReasonNamesTheCurrentStateLabelAndTheEvent(t *testing.T) {
	cases := []struct {
		s    *State
		e    Event
		want string
	}{
		{samples["stopping"], Resume, "Stopping state does not accept the resume event"},
		{samples["queued"], Submit, "Queued state does not accept the submit event"},
		{nil, Abort, "a run that was not submitted does not accept the abort event"},
	}
	for _, c := range cases {
		if _, err := Transition(c.s, c.e, none); err == nil || err.Error() != c.want {
			t.Fatalf("got %v, want %q", err, c.want)
		}
	}
}

func TestObservedWithoutAnObservationHasNoCellAndIsAConflict(t *testing.T) {
	if _, err := Transition(samples["done/up"], Observed, none); err == nil {
		t.Fatal("observed without observation was accepted")
	}
	if _, err := Transition(samples["done/up"], Observed, up); err != nil {
		t.Fatalf("canary: observed up was rejected: %v", err)
	}
}

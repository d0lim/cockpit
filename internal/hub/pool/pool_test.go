package pool

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
)

func rid(n int) string { return fmt.Sprintf("run-20261012-1530%02d-997a37e", n) }

func at(minute int) string { return fmt.Sprintf("2026-10-12T06:%02d:00.000Z", minute) }

func meta(id string, edit ...func(*store.Meta)) store.Meta {
	m := store.Meta{State: lifecycle.State{Phase: lifecycle.Queued, Env: lifecycle.EnvAbsent}, ID: id, Submitter: "cli", CreatedAt: at(0), HostAPI: 1}
	for _, e := range edit {
		e(&m)
	}
	return m
}

func preserved(n, minute int, edit ...func(*store.Meta)) store.Meta {
	slot := n
	base := func(m *store.Meta) {
		m.CreatedAt, m.StartedAt, m.FinishedAt, m.LastUpAt = at(minute), at(minute), at(minute), at(minute)
		m.State = lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictPassed}
		m.Slot = &slot
	}
	return meta(rid(n), append([]func(*store.Meta){base}, edit...)...)
}

func kept(m *store.Meta) { m.Keep = true }

func phase(p lifecycle.Phase) func(*store.Meta) { return func(m *store.Meta) { m.Phase = p } }

func env(e lifecycle.Env) func(*store.Meta) { return func(m *store.Meta) { m.Env = e } }

func view(runs []store.Meta, edit ...func(*View)) View {
	v := View{Runs: runs, Cap: 3, PoolMax: 5}
	for _, e := range edit {
		e(&v)
	}
	return v
}

func ids(runs []store.Meta) []string {
	out := []string{}
	for _, m := range runs {
		out = append(out, m.ID)
	}
	return out
}

func getenv(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }

func TestStopOnDoneIsOnWhenUnsetOrTruthyAndOffOnlyForZeroFalseOff(t *testing.T) {
	on := []map[string]string{{}, {"COCKPIT_STOP_ON_DONE": "1"}, {"COCKPIT_STOP_ON_DONE": "true"}, {"COCKPIT_STOP_ON_DONE": "ON"}, {"COCKPIT_STOP_ON_DONE": "maybe"}}
	off := []map[string]string{{"COCKPIT_STOP_ON_DONE": "0"}, {"COCKPIT_STOP_ON_DONE": "false"}, {"COCKPIT_STOP_ON_DONE": "Off"}}
	for _, kv := range on {
		if !StopOnDoneFrom(getenv(kv)) {
			t.Errorf("%v is off", kv)
		}
	}
	for _, kv := range off {
		if StopOnDoneFrom(getenv(kv)) {
			t.Errorf("%v is on", kv)
		}
	}
}

func TestPoolHoldsUpStoppedStoppingAndResumingRunsOnly(t *testing.T) {
	inside := []store.Meta{preserved(1, 1), preserved(2, 2, env(lifecycle.EnvStopped)), preserved(3, 3, phase(lifecycle.Stopping)), preserved(4, 4, phase(lifecycle.Resuming), env(lifecycle.EnvStopped))}
	outside := []store.Meta{
		meta(rid(5)),
		meta(rid(6), phase(lifecycle.Testing), env(lifecycle.EnvUp)),
		preserved(7, 7, phase(lifecycle.Destroying)),
		preserved(8, 8, env(lifecycle.EnvAbsent)),
		preserved(9, 9, env(lifecycle.EnvDestroyed)),
		preserved(10, 10, env(lifecycle.EnvAbsent), func(m *store.Meta) { m.Mark = lifecycle.Lost }),
	}
	if got := ids(filter(inside, InPool)); !slices.Equal(got, ids(inside)) {
		t.Fatalf("inside %v", got)
	}
	if got := filter(outside, InPool); len(got) != 0 {
		t.Fatalf("outside %v", ids(got))
	}
	all := append(slices.Clone(inside), outside...)
	if got := ids(filter(all, func(m store.Meta) bool { return m.Active() })); !slices.Equal(got, []string{rid(6)}) {
		t.Fatalf("active %v", got)
	}
}

func TestPoolVictimIsTheOldestUnkeptRunOtherThanTheJustFinishedOne(t *testing.T) {
	runs := []store.Meta{preserved(1, 10, kept), preserved(4, 13), preserved(2, 11), preserved(3, 12, kept), preserved(5, 14), preserved(6, 20)}
	if got := Victim(runs, 5); got == nil || got.ID != rid(2) {
		t.Fatalf("%v", got)
	}
}

func TestPoolVictimSkipsTheJustFinishedRunEvenWhenItIsUnkept(t *testing.T) {
	var runs []store.Meta
	for n := 1; n <= 5; n++ {
		runs = append(runs, preserved(n, 10+n, kept))
	}
	if got := Victim(append(runs, preserved(6, 20)), 5); got != nil {
		t.Fatalf("%v", got.ID)
	}
}

func TestPoolVictimIsNoneAtFiveOrFewer(t *testing.T) {
	var runs []store.Meta
	for n := 1; n <= 5; n++ {
		runs = append(runs, preserved(n, n))
	}
	if got := Victim(runs, 5); got != nil {
		t.Fatalf("%v", got.ID)
	}
}

func TestCapVictimsStopsTheOldestLastUpWhenTheActiveShareExceedsTheCap(t *testing.T) {
	three := []store.Meta{preserved(1, 30), preserved(2, 10), preserved(3, 20)}
	if UpCount(view(three), true) != 4 {
		t.Fatal("up count")
	}
	if got := ids(CapVictims(view(three), true)); !slices.Equal(got, []string{rid(2)}) {
		t.Fatalf("%v", got)
	}
	if got := CapVictims(view(three), false); len(got) != 0 {
		t.Fatalf("without the active share: %v", ids(got))
	}
}

func TestForeignComposeProjectsTakeOneSlotEachFromKeptRuns(t *testing.T) {
	two := []store.Meta{preserved(1, 30), preserved(2, 10)}
	if got := CapVictims(view(two), true); len(got) != 0 {
		t.Fatalf("canary: %v", ids(got))
	}
	if UpCount(view(two, func(v *View) { v.ForeignUp = 1 }), false) != UpCount(view(two), false)+1 {
		t.Fatal("a foreign project does not add one")
	}
	if got := ids(CapVictims(view(two, func(v *View) { v.ForeignUp = 1 }), true)); !slices.Equal(got, []string{rid(2)}) {
		t.Fatalf("%v", got)
	}
}

func TestRunningOrphansCountTowardTheCap(t *testing.T) {
	two := []store.Meta{preserved(1, 30), preserved(2, 10)}
	if got := ids(CapVictims(view(two, func(v *View) { v.OrphansUp = 1 }), true)); !slices.Equal(got, []string{rid(2)}) {
		t.Fatalf("%v", got)
	}
}

func TestStoppedAndStoppingRunsAreNotCountedAndResumingRunsCountButAreNotStopped(t *testing.T) {
	runs := []store.Meta{preserved(1, 5, phase(lifecycle.Resuming), env(lifecycle.EnvStopped)), preserved(2, 10), preserved(3, 1, env(lifecycle.EnvStopped)), preserved(4, 2, phase(lifecycle.Stopping))}
	if UpCount(view(runs), true) != 3 {
		t.Fatalf("up %d", UpCount(view(runs), true))
	}
	if got := ids(CapVictims(view(runs, func(v *View) { v.Cap = 2 }), true)); !slices.Equal(got, []string{rid(2)}) {
		t.Fatalf("%v", got)
	}
}

func TestStartBlockNamesTheFirstReasonTheQueueHeadCannotStart(t *testing.T) {
	var allKept, fourKept []store.Meta
	for n := 1; n <= 5; n++ {
		allKept = append(allKept, preserved(n, n, kept))
	}
	fourKept = slices.Clone(allKept[:4])
	unkeptFive := append(slices.Clone(fourKept), preserved(5, 5))
	cases := []struct {
		name   string
		v      View
		active bool
		want   lifecycle.WaitReason
	}{
		{"active run", view(nil), true, lifecycle.WaitActiveRun},
		{"operation in progress", view([]store.Meta{preserved(1, 1, phase(lifecycle.Destroying))}), false, lifecycle.WaitInProgress},
		{"six kept environments", view(append(slices.Clone(unkeptFive), preserved(6, 6))), false, lifecycle.WaitPoolOver},
		{"five with keep on", view(allKept), false, lifecycle.WaitPoolAllKept},
		{"five with one unkept", view(unkeptFive), false, lifecycle.WaitNone},
		{"four kept", view(fourKept), false, lifecycle.WaitNone},
	}
	for _, c := range cases {
		if got := StartBlock(c.v, c.active); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if len(cases) != 6 {
		t.Fatal("case count")
	}
}

func TestResumeCandidatesAreUpKeptRunsByOldestLastUp(t *testing.T) {
	runs := []store.Meta{preserved(1, 30, kept), preserved(2, 10), preserved(3, 20, env(lifecycle.EnvStopped))}
	want := []Candidate{
		{ID: rid(2), LastUpAt: at(10), FinishedAt: at(10), Keep: false, Submitter: "cli"},
		{ID: rid(1), LastUpAt: at(30), FinishedAt: at(30), Keep: true, Submitter: "cli"},
	}
	if got := ResumeCandidates(runs); !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
}

func TestCanResumeRefusesWhenTheResumedRunWouldExceedTheCap(t *testing.T) {
	one := []store.Meta{preserved(1, 1)}
	two := []store.Meta{preserved(1, 1), preserved(2, 2)}
	if !CanResume(view(one), true) || CanResume(view(two), true) || !CanResume(view(two), false) {
		t.Fatal("resume cap")
	}
}

func TestUpCapIsAPositiveIntegerFromTheEnvironmentOrTheDefault(t *testing.T) {
	cases := map[string]int{"4": 4, "": DefaultUpCap, "0": DefaultUpCap, "2.5": DefaultUpCap, "-1": DefaultUpCap}
	for v, want := range cases {
		if got := UpCapFrom(getenv(map[string]string{"COCKPIT_UP_CAP": v})); got != want {
			t.Errorf("%q: got %d, want %d", v, got, want)
		}
	}
}

func TestDefaultLimitsAreFiveKeptTenQueuedAndThreeUp(t *testing.T) {
	if DefaultLimits() != (Limits{PoolMax: 5, QueueMax: 10, UpCap: 3}) {
		t.Fatalf("%+v", DefaultLimits())
	}
}

package lifecycle

import "testing"

var hookStates = map[Key]*State{
	KeyNew:           nil,
	"queued":         {Phase: Queued, Env: EnvAbsent},
	"preparing":      {Phase: Preparing, Env: EnvAbsent},
	"building":       {Phase: Building, Env: EnvAbsent},
	"sealing":        {Phase: Sealing, Env: EnvAbsent},
	"booting":        {Phase: Booting, Env: EnvAbsent},
	"testing":        {Phase: Testing, Env: EnvUp},
	"reporting":      {Phase: Reporting, Env: EnvUp},
	"stopping":       {Phase: Stopping, Env: EnvUp},
	"resuming":       {Phase: Resuming, Env: EnvStopped},
	"destroying":     {Phase: Destroying, Env: EnvUp},
	"done/up":        {Phase: Done, Env: EnvUp},
	"done/stopped":   {Phase: Done, Env: EnvStopped},
	"done/absent":    {Phase: Done, Env: EnvAbsent},
	"done/destroyed": {Phase: Done, Env: EnvDestroyed},
	"lost":           {Phase: Done, Env: EnvAbsent, Mark: Lost},
	"orphan":         {Phase: Done, Env: EnvUp, Mark: Orphan},
}

var expectedHooks = map[string]Hook{
	"preparing destroy": {HookDrain, CauseDestroying}, "preparing fail": {HookDrain, CauseAbort}, "preparing abort": {HookDrain, CauseAbort},
	"building destroy": {HookDrain, CauseDestroying}, "building fail": {HookDrain, CauseAbort}, "building abort": {HookDrain, CauseAbort},
	"sealing destroy": {HookDrain, CauseDestroying}, "sealing fail": {HookDrain, CauseAbort}, "sealing abort": {HookDrain, CauseAbort},
	"booting booted": {HookArm, CauseBooted}, "booting destroy": {HookDrain, CauseDestroying}, "booting fail": {HookDrain, CauseAbort}, "booting abort": {HookDrain, CauseAbort},
	"testing tested": {HookDrain, CauseReporting}, "testing fail": {HookDrain, CauseReporting}, "testing abort": {HookDrain, CauseReporting}, "testing destroy": {HookDrain, CauseDestroying},
	"testing observed/up": {HookDrain, CauseReporting}, "testing observed/stopped": {HookDrain, CauseReporting}, "testing observed/gone": {HookDrain, CauseReporting},
	"reporting reported": {HookArm, CauseKept}, "reporting fail": {HookDrain, CauseAbort}, "reporting abort": {HookDrain, CauseAbort}, "reporting destroy": {HookDrain, CauseDestroying},
	"reporting observed/stopped": {HookDrain, CauseEnvLost}, "reporting observed/gone": {HookDrain, CauseEnvLost},
	"stopping stopped":     {HookDrain, CauseEnvLost},
	"resuming resumed":     {HookArm, CauseResumed},
	"destroying destroyed": {HookDrain, CauseEnvLost},
	"done/up stop":         {HookDrain, CauseStopping}, "done/up destroy": {HookDrain, CauseDestroying},
	"done/up observed/up": {HookArm, CauseReconciled}, "done/up observed/stopped": {HookDrain, CauseEnvLost}, "done/up observed/gone": {HookDrain, CauseEnvLost},
	"done/stopped destroy": {HookDrain, CauseDestroying}, "done/stopped observed/up": {HookArm, CauseReconciled},
}

func TestHooksArmWhenTheEnvironmentComesUpOrStaysUpAfterTheReportAndDrainBeforeReportingStoppingDestroyingOrAbortBothWays(t *testing.T) {
	got := map[string]Hook{}
	transitions := 0
	for key, row := range Transitions {
		before, ok := hookStates[key]
		if !ok {
			t.Fatalf("no representative state for %s", key)
		}
		try := func(name string, event Event, d Detail) {
			after, err := Transition(before, event, d)
			if err != nil {
				return
			}
			transitions++
			if h := HookFor(before, event, d, after); h != (Hook{}) {
				got[string(key)+" "+name] = h
			}
		}
		for event := range row.On {
			try(string(event), event, Detail{Verdict: VerdictPassed})
		}
		for obs := range row.Observed {
			try("observed/"+string(obs), Observed, Detail{Observation: obs})
		}
	}
	for name, want := range expectedHooks {
		if got[name] != want {
			t.Errorf("%s: got %+v, want %+v", name, got[name], want)
		}
	}
	for name, h := range got {
		if _, ok := expectedHooks[name]; !ok {
			t.Errorf("%s fires %+v but is not in the expected table", name, h)
		}
	}
	if transitions < 60 || len(got) != len(expectedHooks) || len(hookStates) != len(Transitions) {
		t.Fatalf("%d transitions, %d hooks, %d expected, %d states for %d rows", transitions, len(got), len(expectedHooks), len(hookStates), len(Transitions))
	}
}

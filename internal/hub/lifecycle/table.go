package lifecycle

type Event string

const (
	Submit       Event = "submit"
	TickStart    Event = "tick-start"
	Prepared     Event = "prepared"
	Built        Event = "built"
	Sealed       Event = "sealed"
	Booted       Event = "booted"
	Tested       Event = "tested"
	Reported     Event = "reported"
	Stop         Event = "stop"
	Stopped      Event = "stopped"
	Resume       Event = "resume"
	Resumed      Event = "resumed"
	ResumeFailed Event = "resume-failed"
	Destroy      Event = "destroy"
	Destroyed    Event = "destroyed"
	Fail         Event = "fail"
	Abort        Event = "abort"
	Reject       Event = "reject"
	Observed     Event = "observed"
)

var Events = []Event{Submit, TickStart, Prepared, Built, Sealed, Booted, Tested, Reported, Stop, Stopped, Resume, Resumed, ResumeFailed, Destroy, Destroyed, Fail, Abort, Reject, Observed}

type Observation string

const (
	ObservedUp      Observation = "up"
	ObservedStopped Observation = "stopped"
	ObservedGone    Observation = "gone"
)

var Observations = []Observation{ObservedUp, ObservedStopped, ObservedGone}

type Detail struct {
	Verdict     Verdict
	Observation Observation
}

type VerdictRule string

const (
	RuleNone    VerdictRule = ""
	RuleAborted VerdictRule = "aborted"
	RuleError   VerdictRule = "error"
	RuleFailure VerdictRule = "failure"
	RuleReport  VerdictRule = "report"
)

type Change struct {
	Phase   Phase
	Env     Env
	Verdict VerdictRule
	Mark    Mark
}

type Row struct {
	On       map[Event]Change
	Observed map[Observation]Change
}

func closeWithoutEnv() map[Observation]Change {
	c := Change{Phase: Done, Env: EnvAbsent, Verdict: RuleError}
	return map[Observation]Change{ObservedUp: c, ObservedStopped: c, ObservedGone: c}
}

func activeRow(next Event, nextPhase Phase, failed Change, aborted Change, observed map[Observation]Change) Row {
	return Row{
		On: map[Event]Change{
			next:    {Phase: nextPhase},
			Fail:    failed,
			Abort:   aborted,
			Destroy: {Phase: Destroying, Verdict: RuleAborted},
		},
		Observed: observed,
	}
}

var noEnvFail = Change{Phase: Done, Env: EnvAbsent, Verdict: RuleFailure}

var noEnvAbort = Change{Phase: Done, Env: EnvAbsent, Verdict: RuleAborted}

var Transitions = map[Key]Row{
	KeyNew: {On: map[Event]Change{Submit: {Phase: Queued, Env: EnvAbsent}}},
	"queued": {On: map[Event]Change{
		TickStart: {Phase: Preparing},
		Destroy:   {Phase: Done, Env: EnvDestroyed, Verdict: RuleAborted},
		Reject:    {Phase: Done, Env: EnvAbsent, Verdict: RuleError},
	}},
	"preparing": activeRow(Prepared, Building, noEnvFail, noEnvAbort, closeWithoutEnv()),
	"building":  activeRow(Built, Sealing, noEnvFail, noEnvAbort, closeWithoutEnv()),
	"sealing":   activeRow(Sealed, Booting, noEnvFail, noEnvAbort, closeWithoutEnv()),
	"booting": {
		On: map[Event]Change{
			Booted:  {Phase: Testing, Env: EnvUp},
			Fail:    {Phase: Done, Env: EnvStopped, Verdict: RuleFailure},
			Abort:   {Phase: Done, Env: EnvStopped, Verdict: RuleAborted},
			Destroy: {Phase: Destroying, Verdict: RuleAborted},
		},
		Observed: map[Observation]Change{
			ObservedUp:      {Phase: Done, Env: EnvStopped, Verdict: RuleError},
			ObservedStopped: {Phase: Done, Env: EnvStopped, Verdict: RuleError},
			ObservedGone:    {Phase: Done, Env: EnvAbsent, Verdict: RuleError},
		},
	},
	"testing": activeRow(Tested, Reporting,
		Change{Phase: Reporting, Verdict: RuleFailure},
		Change{Phase: Reporting, Verdict: RuleAborted},
		map[Observation]Change{
			ObservedUp:      {Phase: Reporting, Verdict: RuleError},
			ObservedStopped: {Phase: Reporting, Env: EnvStopped, Verdict: RuleError},
			ObservedGone:    {Phase: Reporting, Env: EnvAbsent, Verdict: RuleError},
		}),
	"reporting": {
		On: map[Event]Change{
			Reported: {Phase: Done, Verdict: RuleReport},
			Fail:     {Phase: Done, Verdict: RuleFailure},
			Abort:    {Verdict: RuleAborted},
			Destroy:  {Phase: Destroying, Verdict: RuleAborted},
		},
		Observed: map[Observation]Change{
			ObservedUp:      {Verdict: RuleError},
			ObservedStopped: {Env: EnvStopped, Verdict: RuleError},
			ObservedGone:    {Env: EnvAbsent, Verdict: RuleError},
		},
	},
	"stopping": {On: map[Event]Change{Stopped: {Phase: Done, Env: EnvStopped}}},
	"resuming": {On: map[Event]Change{
		Resumed:      {Phase: Done, Env: EnvUp},
		ResumeFailed: {Phase: Done, Env: EnvStopped},
	}},
	"destroying": {On: map[Event]Change{Destroyed: {Phase: Done, Env: EnvDestroyed}}},
	"done/up": {
		On: map[Event]Change{Stop: {Phase: Stopping}, Destroy: {Phase: Destroying}},
		Observed: map[Observation]Change{
			ObservedUp:      {},
			ObservedStopped: {Env: EnvStopped},
			ObservedGone:    {Env: EnvAbsent, Mark: Lost},
		},
	},
	"done/stopped": {
		On: map[Event]Change{Resume: {Phase: Resuming}, Destroy: {Phase: Destroying}},
		Observed: map[Observation]Change{
			ObservedUp:      {Env: EnvUp},
			ObservedStopped: {},
			ObservedGone:    {Env: EnvAbsent, Mark: Lost},
		},
	},
	"done/absent":    {},
	"done/destroyed": {},
	"lost":           {},
	"orphan":         {},
}

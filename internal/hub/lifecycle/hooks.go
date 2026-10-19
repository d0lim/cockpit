package lifecycle

type HookAction string

const (
	HookNone  HookAction = ""
	HookArm   HookAction = "arm"
	HookDrain HookAction = "drain"
)

type HookCause string

const (
	CauseBooted     HookCause = "booted"
	CauseResumed    HookCause = "resumed"
	CauseReconciled HookCause = "reconciled"
	CauseKept       HookCause = "kept"
	CauseReporting  HookCause = "reporting"
	CauseStopping   HookCause = "stopping"
	CauseDestroying HookCause = "destroying"
	CauseAbort      HookCause = "abort"
	CauseEnvLost    HookCause = "env-lost"
)

type Hook struct {
	Action HookAction
	Cause  HookCause
}

var drainPhases = map[Phase]HookCause{Reporting: CauseReporting, Stopping: CauseStopping, Destroying: CauseDestroying}

func HookFor(before *State, event Event, d Detail, after State) Hook {
	if cause, ok := drainPhases[after.Phase]; ok && (before == nil || before.Phase != after.Phase) {
		return Hook{HookDrain, cause}
	}
	switch {
	case event == Abort || event == Fail:
		return Hook{HookDrain, CauseAbort}
	case after.Env == EnvUp && event == Booted:
		return Hook{HookArm, CauseBooted}
	case after.Env == EnvUp && event == Resumed:
		return Hook{HookArm, CauseResumed}
	case after.Env == EnvUp && after.Phase == Done && event == Reported:
		return Hook{HookArm, CauseKept}
	case after.Env == EnvUp && after.Phase == Done && event == Observed && d.Observation == ObservedUp:
		return Hook{HookArm, CauseReconciled}
	case before != nil && before.Env == EnvUp && after.Env != EnvUp:
		return Hook{HookDrain, CauseEnvLost}
	}
	return Hook{}
}

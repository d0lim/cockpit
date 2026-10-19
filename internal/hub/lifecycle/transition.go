package lifecycle

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type Conflict struct {
	Reason string
}

func (c *Conflict) Error() string { return c.Reason }

func tableKey(s *State) Key {
	if s == nil {
		return KeyNew
	}
	return s.Key()
}

func lookup(s *State, event Event, d Detail) (Change, bool) {
	row := Transitions[tableKey(s)]
	if event == Observed {
		c, ok := row.Observed[d.Observation]
		return c, ok
	}
	c, ok := row.On[event]
	return c, ok
}

func nextVerdict(current Verdict, rule VerdictRule, d Detail) Verdict {
	if current != VerdictNone || rule == RuleNone {
		return current
	}
	switch rule {
	case RuleFailure:
		if d.Verdict == VerdictFailed {
			return VerdictFailed
		}
		return VerdictError
	case RuleReport:
		if d.Verdict == VerdictNone {
			return VerdictError
		}
		return d.Verdict
	}
	return Verdict(rule)
}

func pick[T comparable](next, base T) T {
	var zero T
	if next == zero {
		return base
	}
	return next
}

func Transition(s *State, event Event, d Detail) (State, error) {
	change, ok := lookup(s, event, d)
	if !ok {
		where := "a run that was not submitted"
		if s != nil {
			where = Label(*s) + " state"
		}
		return State{}, &Conflict{Reason: fmt.Sprintf("%s does not accept the %s event", where, event)}
	}
	base := State{Phase: Queued, Env: EnvAbsent}
	if s != nil {
		base = *s
	}
	return State{
		Phase:   pick(change.Phase, base.Phase),
		Env:     pick(change.Env, base.Env),
		Verdict: nextVerdict(base.Verdict, change.Verdict, d),
		Mark:    pick(change.Mark, base.Mark),
	}, nil
}

func AllowedEvents(s *State, served bool) []Event {
	row := Transitions[tableKey(s)]
	var out []Event
	for _, e := range Events {
		if e == Resume && !served {
			continue
		}
		if e == Observed && len(row.Observed) > 0 {
			out = append(out, e)
			continue
		}
		if _, ok := row.On[e]; ok {
			out = append(out, e)
		}
	}
	return out
}

func Serves(served []int, api int) bool {
	return slices.Contains(served, api)
}

func HostAPINotice(api int, served []int, cockpitVersion string) string {
	parts := make([]string, len(served))
	for i, v := range served {
		parts[i] = strconv.Itoa(v)
	}
	return fmt.Sprintf("unknown host API version %d: this hub serves host API %s (cockpit %s)", api, strings.Join(parts, ", "), cockpitVersion)
}

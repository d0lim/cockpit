package wire

import (
	"cmp"
	"slices"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
)

const runnerCaller = "cockpit-runner"

type stepWindow struct {
	step  int
	start time.Time
	end   time.Time
}

type stepTimes struct {
	bySpan  map[string]int
	windows []stepWindow
}

func LinkCalls(calls []Call, result *contract.Result) []Link {
	steps := readSteps(result)
	order := make([]int, len(calls))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Or(calls[a].Started.Compare(calls[b].Started), cmp.Compare(calls[a].Seq, calls[b].Seq))
	})
	links := make([]Link, len(calls))
	serving := map[string][]int{}
	for _, i := range order {
		c := calls[i]
		links[i] = linkCall(c, steps, calls, links, serving[c.Caller])
		if kind := links[i].Kind; kind == LinkExact || kind == LinkNested {
			serving[c.Target] = append(serving[c.Target], i)
		}
	}
	return links
}

func linkCall(c Call, steps stepTimes, calls []Call, links []Link, candidates []int) Link {
	if step, ok := steps.bySpan[c.Span]; ok && c.Span != "" {
		return Link{Kind: LinkExact, Step: step}
	}
	step, inside := steps.at(c.Started)
	if inside && c.Caller == runnerCaller {
		return Link{Kind: LinkExact, Step: step}
	}
	if parent, ok := tightestParent(c, calls, candidates); ok {
		return Link{Kind: LinkNested, Step: links[parent].Step, Parent: calls[parent].Seq}
	}
	if inside {
		return Link{Kind: LinkTime, Step: step}
	}
	return Link{Kind: LinkNone, Step: NoStep}
}

func tightestParent(c Call, calls []Call, candidates []int) (int, bool) {
	best, found := 0, false
	for _, i := range candidates {
		if encloses(calls[i], c) && (!found || !tighter(calls[best], calls[i])) {
			best, found = i, true
		}
	}
	return best, found
}

func encloses(parent, c Call) bool {
	if parent.Started.After(c.Started) {
		return false
	}
	if parent.Finished.IsZero() {
		return true
	}
	return !c.Finished.IsZero() && !answeredAt(c).After(parent.Finished)
}

func answeredAt(c Call) time.Time {
	if c.Replied.IsZero() {
		return c.Finished
	}
	return c.Replied
}

func tighter(a, b Call) bool {
	if a.Finished.IsZero() || b.Finished.IsZero() {
		return !a.Finished.IsZero()
	}
	return a.Finished.Sub(a.Started) < b.Finished.Sub(b.Started)
}

func readSteps(result *contract.Result) stepTimes {
	steps := stepTimes{bySpan: map[string]int{}}
	if result == nil {
		return steps
	}
	for i, s := range result.Steps {
		for _, call := range s.Calls {
			if _, taken := steps.bySpan[call.SpanID]; call.SpanID != "" && !taken {
				steps.bySpan[call.SpanID] = i
			}
		}
		start, err := time.Parse(time.RFC3339Nano, s.StartedAt)
		if err != nil {
			continue
		}
		end, err := time.Parse(time.RFC3339Nano, s.FinishedAt)
		if err != nil {
			end = time.Time{}
		}
		steps.windows = append(steps.windows, stepWindow{step: i, start: start, end: end})
	}
	slices.SortStableFunc(steps.windows, func(a, b stepWindow) int { return a.start.Compare(b.start) })
	return steps
}

func (s stepTimes) at(t time.Time) (int, bool) {
	for i := len(s.windows) - 1; i >= 0; i-- {
		w := s.windows[i]
		if w.start.After(t) {
			continue
		}
		if w.end.IsZero() || !t.Truncate(time.Millisecond).After(w.end) {
			return w.step, true
		}
	}
	return NoStep, false
}

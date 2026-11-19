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

type linker struct {
	calls   []Call
	steps   stepTimes
	chains  spanChains
	links   []Link
	tied    []bool
	linked  []bool
	serving map[string][]int
}

func LinkCalls(calls []Call, result *contract.Result, spans []Span) []Link {
	order := make([]int, len(calls))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Or(calls[a].Started.Compare(calls[b].Started), cmp.Compare(calls[a].Seq, calls[b].Seq))
	})
	l := linker{
		calls: calls, steps: readSteps(result), chains: newSpanChains(calls, order, spans), links: make([]Link, len(calls)),
		tied: make([]bool, len(calls)), linked: make([]bool, len(calls)), serving: map[string][]int{},
	}
	for _, i := range order {
		l.links[i], l.tied[i] = l.link(i)
		l.linked[i] = true
		if canServe(l.links[i].Kind) {
			l.serving[calls[i].Target] = append(l.serving[calls[i].Target], i)
		}
	}
	return l.links
}

func canServe(kind LinkKind) bool {
	return kind == LinkExact || kind == LinkTraced || kind == LinkNested || kind == LinkBackground
}

func (l linker) link(i int) (Link, bool) {
	c := l.calls[i]
	if step, ok := l.steps.bySpan[c.Span]; ok && c.Span != "" {
		return Link{Kind: LinkExact, Step: step}, true
	}
	step, inside := l.steps.at(c.Started)
	if inside && c.Caller == runnerCaller {
		return Link{Kind: LinkExact, Step: step}, true
	}
	window := NoStep
	if inside {
		window = step
	}
	end := l.chains.reach(i)
	if end.found && l.linked[end.parent] {
		link := l.below(LinkTraced, end.parent, window)
		link.Route = end.route
		return link, l.tied[end.parent]
	}
	if end.rooted {
		return Link{Kind: LinkBackground, Step: window, Root: end.root}, false
	}
	if parent, ok := tightestParent(c, l.calls, l.serving[c.Caller]); ok {
		return l.below(LinkNested, parent, window), l.tied[parent]
	}
	if inside {
		return Link{Kind: LinkTime, Step: step}, false
	}
	return Link{Kind: LinkNone, Step: NoStep}, false
}

func (l linker) below(kind LinkKind, parent, window int) Link {
	step := window
	if l.tied[parent] || window == NoStep {
		step = l.links[parent].Step
	}
	return Link{Kind: kind, Step: step, Parent: l.calls[parent].Seq}
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

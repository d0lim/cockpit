package screen

import (
	"net/url"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
)

const (
	onlyFailed  = "failed"
	onlySkipped = "skipped"
	paneSource  = "source"
)

type treeStep struct {
	id     string
	title  string
	status string
	meta   string
	href   string
	on     bool
}

type treeFlow struct {
	id     string
	title  string
	status string
	count  string
	href   string
	on     bool
	steps  []treeStep
}

type treeStage struct {
	name  string
	flows []treeFlow
}

type treeFilter struct {
	label string
	count int
	href  string
	on    bool
}

type treeOutside struct {
	href  string
	calls int
	on    bool
}

type flowTree struct {
	stages  []treeStage
	filters []treeFilter
	outside *treeOutside
	rows    int
}

type treePick struct {
	flow string
	step string
}

type treeLinks struct {
	flow   func(flow string) string
	step   func(flow, step string) string
	filter func(only string) string
	meta   func(flow string, s contract.FlowStep) string
}

func (st *state) only() string {
	switch v := st.query.Get("only"); v {
	case onlyFailed, onlySkipped:
		return v
	}
	return ""
}

type flowsRead struct {
	done   bool
	source string
	flows  []contract.FlowSummary
	err    error
}

func (st *state) flows() (string, []contract.FlowSummary, error) {
	if !st.flowsMemo.done {
		source, flows, err := st.s.d.Reader.Flows(st.run.ID)
		st.flowsMemo = flowsRead{done: true, source: source, flows: flows, err: err}
	}
	return st.flowsMemo.source, st.flowsMemo.flows, st.flowsMemo.err
}

func (st *state) treeFlows() []contract.FlowSummary {
	_, flows, err := st.flows()
	if err != nil {
		return nil
	}
	return flows
}

func stepShown(only string, s contract.FlowStep) bool {
	switch only {
	case onlyFailed:
		return s.Status == contract.StepStatusFailed
	case onlySkipped:
		return s.Status == contract.StepStatusSkipped
	}
	return true
}

func buildTree(flows []contract.FlowSummary, only string, pick treePick, links treeLinks) flowTree {
	var t flowTree
	all, failed, skipped := 0, 0, 0
	for _, f := range flows {
		all += f.Steps
		failed += f.Failed
		skipped += f.Skipped
		row := treeFlow{id: f.ID, title: f.Title, status: string(f.Status), count: itoa(f.Passed) + " / " + itoa(f.Steps), href: links.flow(f.ID), on: pick.flow == f.ID && pick.step == ""}
		if only != "" || pick.flow == f.ID {
			for _, s := range f.StepList {
				if !stepShown(only, s) {
					continue
				}
				row.steps = append(row.steps, treeStep{
					id: s.ID, title: s.Title, status: string(s.Status), meta: links.meta(f.ID, s),
					href: links.step(f.ID, s.ID), on: pick.flow == f.ID && pick.step == s.ID,
				})
			}
		}
		if only != "" && len(row.steps) == 0 {
			continue
		}
		if len(t.stages) == 0 || t.stages[len(t.stages)-1].name != f.Stage {
			t.stages = append(t.stages, treeStage{name: f.Stage})
		}
		last := &t.stages[len(t.stages)-1]
		last.flows = append(last.flows, row)
		t.rows += 1 + len(row.steps)
	}
	t.filters = []treeFilter{
		{label: "All", count: all, href: links.filter(""), on: only == ""},
		{label: "Failed", count: failed, href: links.filter(onlyFailed), on: only == onlyFailed},
		{label: "Skipped", count: skipped, href: links.filter(onlySkipped), on: only == onlySkipped},
	}
	return t
}

func treeKey(pick treePick) string { return "tree-" + pick.flow + "/" + pick.step }

func (st *state) flowQuery(step, pane string) string {
	q := url.Values{}
	if step != "" {
		q.Set("step", step)
	}
	if pane != "" {
		q.Set("pane", pane)
	}
	if only := st.only(); only != "" {
		q.Set("only", only)
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

func (st *state) flowHref(flow, step, pane string) string {
	return "/runs/" + st.run.ID + "/report/" + flow + st.flowQuery(step, pane)
}

func (st *state) pane() string {
	if st.query.Get("pane") == paneSource {
		return paneSource
	}
	return ""
}

func stepTook(_ string, s contract.FlowStep) string {
	if s.DurationMs == nil {
		return ""
	}
	return durationText(s.DurationMs)
}

func (st *state) stepTree(pick treePick) flowTree {
	pane := st.pane()
	return buildTree(st.treeFlows(), st.only(), pick, treeLinks{
		flow: func(flow string) string { return st.flowHref(flow, "", pane) },
		step: func(flow, step string) string { return st.flowHref(flow, step, pane) },
		filter: func(only string) string {
			set := map[string][]string{"only": nil}
			if only != "" {
				set["only"] = []string{only}
			}
			return st.href(set)
		},
		meta: stepTook,
	})
}

func flowsOfTake(steps []api.WireStep, suite []contract.FlowSummary) []contract.FlowSummary {
	var out []contract.FlowSummary
	for _, s := range steps {
		if len(out) == 0 || out[len(out)-1].ID != s.Flow {
			f := contract.FlowSummary{ID: s.Flow}
			if known := findFlow(suite, s.Flow); known != nil {
				f.Stage, f.Title = known.Stage, known.Title
			}
			out = append(out, f)
		}
		f := &out[len(out)-1]
		f.StepList = append(f.StepList, contract.FlowStep{ID: s.Step, Title: s.Title, Status: s.Status})
		f.Steps++
		switch s.Status {
		case contract.StepStatusPassed:
			f.Passed++
		case contract.StepStatusFailed:
			f.Failed++
		case contract.StepStatusSkipped:
			f.Skipped++
		}
	}
	for i := range out {
		out[i].Status = flowStatusOf(out[i])
	}
	return out
}

func flowStatusOf(f contract.FlowSummary) contract.FlowOutcome {
	switch {
	case f.Failed > 0:
		return contract.FlowOutcomeFailed
	case f.Steps > 0 && f.Passed == f.Steps:
		return contract.FlowOutcomePassed
	case f.Steps > 0 && f.Skipped == f.Steps:
		return contract.FlowOutcomeSkipped
	}
	return contract.FlowOutcomeIncomplete
}

func (st *state) takeFlows(v api.WireView) []contract.FlowSummary {
	return flowsOfTake(v.Steps, st.treeFlows())
}

func (st *state) callTree(v api.WireView) flowTree {
	flows := st.takeFlows(v)
	calls := map[treePick]int{}
	for _, s := range v.Steps {
		calls[treePick{s.Flow, s.Step}] = s.Calls()
	}
	pick := treePick{}
	switch v.Scope {
	case api.WireScopeStep:
		pick = treePick{v.Flow, v.Step}
	case api.WireScopeFlow:
		pick = treePick{flow: v.Flow}
	}
	t := buildTree(flows, st.only(), pick, treeLinks{
		flow: st.wireFlowHref,
		step: func(flow, step string) string { return st.wireStepHref(api.WireStep{Flow: flow, Step: step}) },
		filter: func(only string) string {
			set := map[string][]string{"only": nil}
			if only != "" {
				set["only"] = []string{only}
			}
			return st.href(set)
		},
		meta: func(flow string, s contract.FlowStep) string { return wireCallsShort(calls[treePick{flow, s.ID}]) },
	})
	t.outside = &treeOutside{href: st.wireOutsideHref(), calls: v.Outside, on: v.Scope == api.WireScopeOutside}
	return t
}

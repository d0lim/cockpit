package api

import (
	"cmp"
	"slices"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/wire"
)

func (r *Reader) Wire(id string, q WireQuery) (WireView, error) {
	run, err := r.wireRun(id)
	if err != nil {
		return WireView{}, err
	}
	standing, err := r.wireStanding(run)
	if err != nil {
		return WireView{}, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = WirePageSize
	}
	view := WireView{
		ID: run.view.ID, WireStanding: standing, Steps: []WireStep{}, Routes: []WireRouteCalls{}, Calls: []WireHead{},
		Offset: max(q.Offset, 0), Limit: min(limit, wirePageMax),
	}
	if len(standing.Takes) == 0 {
		return view, nil
	}
	view.Take = q.Take
	if view.Take == "" {
		view.Take = defaultTake(standing.Takes)
	}
	rec, err := r.wireRecord(run, view.Take)
	if err != nil {
		return WireView{}, err
	}
	status := rec.index.Status
	view.Record, view.Outside = &status, len(rec.outside)
	view.Steps, view.Routes = wireSteps(rec, run.mask), wireRoutes(rec, run.mask)
	rows, err := pickWireRows(&view, rec, q)
	if err != nil {
		return WireView{}, err
	}
	if rows, err = filterWireTarget(rec, rows, view.Routes, q.Target); err != nil {
		return WireView{}, err
	}
	view.Total = len(rows)
	start := min(view.Offset, len(rows))
	for _, i := range rows[start:min(start+view.Limit, len(rows))] {
		view.Calls = append(view.Calls, wireHead(rec, i, run.mask))
	}
	return view, nil
}

func wireSteps(rec *wireRecord, m *wireMask) []WireStep {
	out := make([]WireStep, len(rec.index.Steps))
	for i, ref := range rec.index.Steps {
		c := rec.steps[i]
		out[i] = WireStep{
			Flow: m.text(ref.Flow), Step: m.text(ref.Step), Title: m.text(rec.titles[wireStepKey{ref.Flow, ref.Step}]), Status: ref.Status,
			Exact: c.exact, Traced: c.traced, Nested: c.nested, Background: c.background, Time: c.timed,
		}
	}
	return out
}

func wireRoutes(rec *wireRecord, m *wireMask) []WireRouteCalls {
	out := make([]WireRouteCalls, len(rec.index.Routes))
	for i, route := range rec.index.Routes {
		out[i] = WireRouteCalls{Name: m.text(string(route.Name)), Service: m.text(route.Service), Port: route.Port, Calls: rec.routes[i]}
	}
	return out
}

func pickWireRows(view *WireView, rec *wireRecord, q WireQuery) ([]int, error) {
	switch {
	case q.Scope != "" && q.Scope != WireScopeOutside && q.Scope != WireScopeFlow:
		return nil, badRequest("scope is outside, flow or empty")
	case q.Scope == WireScopeOutside && (q.Flow != "" || q.Step != ""):
		return nil, badRequest("scope=outside takes no flow or step")
	case q.Scope == WireScopeFlow && (q.Flow == "" || q.Step != ""):
		return nil, badRequest("scope=flow takes a flow and no step")
	case q.Scope == WireScopeFlow:
		return pickWireFlow(view, rec, q.Flow)
	case (q.Flow == "") != (q.Step == ""):
		return nil, badRequest("flow and step come together")
	case q.Scope == WireScopeOutside:
		view.Scope = WireScopeOutside
		return rec.outside, nil
	}
	at := -1
	if q.Flow != "" {
		at = slices.IndexFunc(view.Steps, func(s WireStep) bool { return s.Flow == q.Flow && s.Step == q.Step })
		if at < 0 {
			return nil, notFound("this take has no such step")
		}
	} else {
		at = defaultWireStep(view.Steps)
	}
	if at < 0 {
		if len(rec.outside) > 0 {
			view.Scope = WireScopeOutside
		}
		return rec.outside, nil
	}
	view.Scope, view.Flow, view.Step = WireScopeStep, view.Steps[at].Flow, view.Steps[at].Step
	return rec.steps[at].heads, nil
}

func pickWireFlow(view *WireView, rec *wireRecord, flow string) ([]int, error) {
	var rows []int
	found := false
	for i, s := range view.Steps {
		if s.Flow == flow {
			found = true
			rows = append(rows, rec.steps[i].heads...)
		}
	}
	if !found {
		return nil, notFound("this take has no such flow")
	}
	heads := rec.index.Heads
	slices.SortStableFunc(rows, func(a, b int) int {
		return cmp.Or(heads[a].Started.Compare(heads[b].Started), cmp.Compare(heads[a].Seq, heads[b].Seq))
	})
	view.Scope, view.Flow = WireScopeFlow, flow
	return rows, nil
}

func defaultWireStep(steps []WireStep) int {
	if at := slices.IndexFunc(steps, func(s WireStep) bool { return s.Status == contract.StepStatusFailed }); at >= 0 {
		return at
	}
	return slices.IndexFunc(steps, func(s WireStep) bool { return s.Calls() > 0 })
}

func filterWireTarget(rec *wireRecord, rows []int, routes []WireRouteCalls, target string) ([]int, error) {
	if target == "" {
		return rows, nil
	}
	at := slices.IndexFunc(routes, func(r WireRouteCalls) bool { return r.Name == target })
	if at < 0 {
		return nil, notFound("this take has no such route")
	}
	route := rec.index.Routes[at]
	var out []int
	for _, i := range rows {
		if routeTakes(route.Service, route.Port, rec.index.Heads[i]) {
			out = append(out, i)
		}
	}
	return out, nil
}

func wireHead(rec *wireRecord, i int, m *wireMask) WireHead {
	h := rec.index.Heads[i]
	head := WireHead{
		Seq: h.Seq, Caller: m.text(h.Caller), Target: m.text(h.Target), Port: h.Port, Method: m.text(h.Method), Path: m.text(h.Path),
		Status: h.Status, Outcome: h.Outcome, Withheld: h.Withheld, Started: h.Started, Replied: h.Replied, Finished: h.Finished,
		DurationMs: h.DurationMs, Link: h.Link.Kind, Parent: h.Link.Parent, Route: h.Link.Route, Root: m.text(h.Link.Root), Depth: rec.depth[i],
	}
	if s := h.Link.Step; h.Link.Kind != wire.LinkNone && s >= 0 && s < len(rec.index.Steps) {
		head.Step = m.text(rec.index.Steps[s].Step)
	}
	return head
}

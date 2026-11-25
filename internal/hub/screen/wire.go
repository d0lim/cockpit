package screen

import (
	"cmp"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/wire"
)

const (
	wireCallPanelID = "wire-call"
	wireFlowHint    = "Every step of this flow, in the order the calls started."
	wireNoRecord    = "This run did not record its service calls: it was submitted without wire, so no flow or step has Calls."
)

func wireNotHeld(err error) bool {
	var p *api.Problem
	return errors.As(err, &p) && p.Status == http.StatusNotFound
}

func wireRecorded(st *state) (int, string) {
	if !st.run.Wire {
		return http.StatusNotFound, "This run was submitted without wire, so it has no record of its service calls."
	}
	return http.StatusOK, ""
}

func (st *state) wireQuery() api.WireQuery {
	q := st.query
	offset, _ := strconv.Atoi(q.Get("offset"))
	return api.WireQuery{
		Take: q.Get("take"), Flow: q.Get("flow"), Step: q.Get("step"), Scope: q.Get("scope"),
		Target: q.Get("target"), Offset: max(offset, 0),
	}
}

func (st *state) wireView() (api.WireView, error) {
	return st.s.d.Reader.Wire(st.run.ID, st.wireQuery())
}

type wireNote struct {
	tone Tone
	text string
}

func wireNotes(v api.WireView) []wireNote {
	var out []wireNote
	switch {
	case v.Status == api.WirePending:
		out = append(out, wireNote{ToneInfo, "The record is fixed when the run reaches its report."})
	case v.Status == api.WireNone && v.State == "":
		out = append(out, wireNote{ToneWarn, "This run ended without reaching its report, so no record was taken."})
	}
	if v.State != "" && v.State != wire.StateTaken {
		out = append(out, wireNote{ToneWarn, "The record of the suite was not stored (" + string(v.State) + "): " + v.Detail + "."})
	}
	if v.Discarded {
		out = append(out, wireNote{ToneWarn, "The redaction rescan discarded the report bundle. This record is kept outside it and still opens."})
	}
	if v.Record != nil && v.Record.WithheldCalls > 0 {
		out = append(out, wireNote{ToneWarn, wireCount(v.Record.WithheldCalls) + " failed the check that nothing secret remains, so only the caller, target, method, status and times were kept."})
	}
	if v.Record != nil && v.Record.CallsDropped > 0 {
		out = append(out, wireNote{ToneWarn, wireCount(v.Record.CallsDropped) + " did not fit the record and are missing from it."})
	}
	if silent := silentRoutes(v.Routes); len(silent) > 0 {
		out = append(out, wireNote{ToneInfo, "No call went through " + strings.Join(silent, ", ") + ". A service reaches a route only through its address variable, so check that its address reads it."})
	}
	return out
}

func wireCount(n int) string {
	if n == 1 {
		return "1 call"
	}
	return itoa(n) + " calls"
}

func silentRoutes(routes []api.WireRouteCalls) []string {
	var out []string
	for _, r := range routes {
		if r.Calls == 0 {
			out = append(out, r.Name+" ("+contract.WireVariable(r.Name)+")")
		}
	}
	return out
}

func (st *state) wireRunNote() (wireNote, bool) {
	if !st.run.Wire {
		return wireNote{ToneIdle, wireNoRecord}, true
	}
	const lead = "This run's service calls passed through the wire recorder. "
	s, err := st.s.d.Reader.WireStanding(st.run.ID)
	switch {
	case err != nil:
		return wireNote{ToneInfo, lead + "Its record cannot be read now."}, true
	case s.Status == api.WirePending:
		return wireNote{ToneInfo, lead + "The record is fixed at the report."}, true
	case s.State == wire.StateTaken:
		return wireNote{ToneInfo, lead + "The record is stored."}, true
	case s.State != "":
		return wireNote{ToneWarn, lead + "The record of the suite is " + string(s.State) + ": " + s.Detail + "."}, true
	case s.Status == api.WireReady:
		return wireNote{ToneInfo, lead + "The takes of its execs are stored."}, true
	}
	return wireNote{ToneWarn, lead + "No record was taken."}, true
}

func wireStepOn(v api.WireView, s api.WireStep) bool {
	return v.Scope == api.WireScopeStep && v.Flow == s.Flow && v.Step == s.Step
}

func (st *state) wireStepHref(s api.WireStep) string {
	return st.href(map[string][]string{"flow": {s.Flow}, "step": {s.Step}, "scope": nil, "offset": nil, "call": nil, "open": nil})
}

func (st *state) wireOutsideHref() string {
	return st.href(map[string][]string{"scope": {api.WireScopeOutside}, "flow": nil, "step": nil, "offset": nil, "call": nil, "open": nil})
}

func (st *state) wireTakeHref(take string) string {
	return st.href(map[string][]string{"take": {take}, "flow": nil, "step": nil, "scope": nil, "target": nil, "offset": nil, "call": nil, "open": nil})
}

func (st *state) wireTargetHref(target string) string {
	set := map[string][]string{"target": nil, "offset": nil, "call": nil, "open": nil}
	if target != "" {
		set["target"] = []string{target}
	}
	return st.href(set)
}

func (st *state) wireCallHref(seq int64) string {
	return st.href(map[string][]string{"call": {strconv.FormatInt(seq, 10)}})
}

func (st *state) wireCloseHref() string {
	return st.href(map[string][]string{"call": nil})
}

func wireScrollKey(seq int64) string {
	return "call-" + strconv.FormatInt(seq, 10)
}

func wireTakeOrder(takes []string) []string {
	out := slices.Clone(takes)
	slices.SortStableFunc(out, func(a, b string) int {
		return cmp.Compare(wireTakeRank(a), wireTakeRank(b))
	})
	return out
}

func wireTakeRank(take string) int {
	if take == wire.SuiteTake {
		return 0
	}
	return 1
}

func (st *state) wireOffsetHref(offset int) string {
	return st.href(map[string][]string{"offset": {itoa(max(offset, 0))}, "call": nil, "open": nil})
}

func wireStepHeading(v api.WireView) (string, string) {
	if v.Scope == api.WireScopeOutside {
		return "Outside any step", "Calls that started while no step was running."
	}
	if v.Scope == api.WireScopeFlow {
		return v.Flow, wireFlowHint
	}
	if v.Scope != api.WireScopeStep {
		return "No calls", "This take holds no call."
	}
	at := slices.IndexFunc(v.Steps, func(s api.WireStep) bool { return wireStepOn(v, s) })
	if at < 0 {
		return v.Flow + " › " + v.Step, ""
	}
	s := v.Steps[at]
	return s.Flow + " › " + s.Step, s.Title
}

func wirePageText(v api.WireView) string {
	if v.Total == 0 {
		return "No calls here."
	}
	if len(v.Calls) == 0 {
		return "Past the last of " + itoa(v.Total) + " calls."
	}
	return "Calls " + itoa(v.Offset+1) + " to " + itoa(v.Offset+len(v.Calls)) + " of " + itoa(v.Total)
}

type wireRow struct {
	head   api.WireHead
	offset string
	band   string
}

func wireTimedHeading(v api.WireView) string {
	if v.Scope == api.WireScopeFlow {
		return "During this flow, not tied to its steps"
	}
	return "During this step, not tied to it"
}

func wireBands(v api.WireView, rows []wireRow) []wireRow {
	if v.Scope != api.WireScopeFlow {
		return rows
	}
	titles := wireStepTitles(v)
	for i := range rows {
		if i > 0 && rows[i].head.Step == rows[i-1].head.Step {
			continue
		}
		rows[i].band = "Outside any step"
		if s := rows[i].head.Step; s != "" {
			rows[i].band = wireStepLabel(titles, s)
		}
	}
	return rows
}

func wireRows(calls []api.WireHead) (linked, timed []wireRow) {
	if len(calls) == 0 {
		return nil, nil
	}
	first := calls[0].Started
	row := func(c api.WireHead) wireRow {
		ms := c.Started.Sub(first).Milliseconds()
		return wireRow{head: c, offset: "+" + durationText(&ms)}
	}
	shown := map[int64]bool{}
	for _, c := range calls {
		shown[c.Seq] = api.WireLinkTied(c.Link)
	}
	children := map[int64][]api.WireHead{}
	var roots []api.WireHead
	for _, c := range calls {
		switch {
		case c.Link == wire.LinkTime || c.Link == wire.LinkBackground:
			timed = append(timed, row(c))
		case c.Parent != 0 && shown[c.Parent]:
			children[c.Parent] = append(children[c.Parent], c)
		default:
			roots = append(roots, c)
		}
	}
	var walk func(c api.WireHead)
	walk = func(c api.WireHead) {
		linked = append(linked, row(c))
		for _, child := range children[c.Seq] {
			walk(child)
		}
	}
	for _, c := range roots {
		walk(c)
	}
	return linked, timed
}

var wireIndents = []string{"", "pl-4", "pl-8", "pl-12", "pl-16"}

func wireIndent(depth int) string { return wireIndents[min(depth, len(wireIndents)-1)] }

func wireDuration(h api.WireHead) string {
	if h.Outcome == wire.OutcomeIncomplete && h.DurationMs == 0 {
		return durationText(nil)
	}
	return durationText(&h.DurationMs)
}

var wireOutcomes = map[wire.Outcome]string{
	wire.OutcomeAnswered:   "answered",
	wire.OutcomeNoUpstream: "no upstream",
	wire.OutcomeCallerGone: "caller left",
	wire.OutcomeIncomplete: "incomplete",
}

func wireResult(h api.WireHead) (Tone, string) {
	switch {
	case h.Status == 0:
		return ToneWarn, wireOutcomes[h.Outcome]
	case h.Status >= 400:
		return ToneBad, itoa(h.Status)
	}
	return ToneOK, itoa(h.Status)
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

func wireStamp(t time.Time) string { return t.Local().Format("01-02 15:04:05.000") }

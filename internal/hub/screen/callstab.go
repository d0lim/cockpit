package screen

import (
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/wire"
)

const (
	callsNoWire     = "Not recorded: this run was submitted without wire."
	callsStepIdle   = "This step did not run, so it made no call."
	callsFlowIdle   = "This flow did not run, so it made no call."
	callsUnread     = "The record cannot be read now."
	callsPending    = "The record is fixed when the run reaches its report."
	callsNotStored  = "The record of the suite was not stored (<state>)."
	callsNotTaken   = "No record of the suite was taken."
	callsNotListed  = "The record does not list it."
	callsStepSilent = "No call was recorded during this step."
	callsFlowSilent = "No call was recorded during this flow."
)

var callsLines = []string{
	callsNoWire, callsStepIdle, callsFlowIdle, callsUnread, callsPending,
	callsNotStored, callsNotTaken, callsNotListed, callsStepSilent, callsFlowSilent,
}

type callsTab struct {
	open    bool
	counted bool
	count   int
	why     string
}

type suiteWireRead struct {
	done bool
	view api.WireView
	err  error
}

func (st *state) suiteWire() (api.WireView, error) {
	if !st.suiteMemo.done {
		st.suiteMemo = suiteWireRead{done: true}
		s, err := st.s.d.Reader.WireStanding(st.run.ID)
		switch {
		case err != nil:
			st.suiteMemo.err = err
		case s.State != wire.StateTaken:
			st.suiteMemo.view = api.WireView{WireStanding: s}
		default:
			st.suiteMemo.view, st.suiteMemo.err = st.s.d.Reader.Wire(st.run.ID, api.WireQuery{Take: wire.SuiteTake, Limit: 1})
		}
	}
	return st.suiteMemo.view, st.suiteMemo.err
}

func stepCalls(steps []api.WireStep, flow, step string) (int, bool) {
	n, listed := 0, false
	for _, s := range steps {
		if s.Flow == flow && (step == "" || s.Step == step) {
			n += s.Calls()
			listed = true
		}
	}
	return n, listed
}

func (st *state) callsTab(f contract.FlowSummary, s *contract.FlowStep) callsTab {
	idle, silent := callsFlowIdle, callsFlowSilent
	if s != nil {
		idle, silent = callsStepIdle, callsStepSilent
	}
	if !st.run.Wire {
		return callsTab{why: callsNoWire}
	}
	if !ranIn(f, s) {
		return callsTab{why: idle}
	}
	v, err := st.suiteWire()
	switch {
	case err != nil:
		return callsTab{why: callsUnread}
	case v.Status == api.WirePending:
		return callsTab{why: callsPending}
	case v.State != "" && v.State != wire.StateTaken:
		return callsTab{why: strings.Replace(callsNotStored, "<state>", string(v.State), 1)}
	case v.State == "":
		return callsTab{why: callsNotTaken}
	}
	n, listed := stepCalls(v.Steps, f.ID, stepID(s))
	switch {
	case !listed:
		return callsTab{why: callsNotListed}
	case n == 0:
		return callsTab{counted: true, why: silent}
	}
	return callsTab{open: true, counted: true, count: n}
}

func wireTab(v api.WireView, flow, step string) callsTab {
	n, listed := stepCalls(v.Steps, flow, step)
	return callsTab{open: true, counted: listed, count: n}
}

func (st *state) stepCallsNote(flow string, s contract.FlowStep) string {
	if !st.run.Wire || s.Status == contract.StepStatusSkipped {
		return ""
	}
	v, err := st.suiteWire()
	if err != nil || v.State != wire.StateTaken {
		return ""
	}
	n, listed := stepCalls(v.Steps, flow, s.ID)
	if !listed {
		return ""
	}
	return wireCallsShort(n)
}

func wireCallsShort(n int) string {
	if n == 0 {
		return "no calls"
	}
	return wireCount(n)
}

func (st *state) callStep(c api.WireCallView) (*contract.FlowSummary, *contract.FlowStep) {
	f := findFlow(st.treeFlows(), c.Flow)
	if f == nil {
		return nil, nil
	}
	return f, pickedStep(*f, c.Step)
}

func callStepLead(h api.WireHead) string {
	if api.WireLinkTied(h.Link) {
		return "in"
	}
	return "during"
}

package screen

import (
	"slices"
	"strconv"

	"github.com/d0lim/cockpit/internal/hub/api"
)

func (st *state) wireViewName() string {
	if name := st.query.Get("view"); slices.Contains(wireViewNames, name) {
		return name
	}
	return wireViewSequence
}

func (st *state) wireViewHref(name string) string {
	return st.href(map[string][]string{"view": {name}})
}

func (st *state) wireTiedOnly() bool { return st.query.Get("ties") == wireTiesOnly }

func (st *state) wireTiesHref(only bool) string {
	set := map[string][]string{"ties": nil, "open": nil}
	if only {
		set["ties"] = []string{wireTiesOnly}
	}
	return st.href(set)
}

func (st *state) wireFlowHref(flow string) string {
	return st.href(map[string][]string{"scope": {api.WireScopeFlow}, "flow": {flow}, "step": nil, "offset": nil, "call": nil, "open": nil})
}

func (st *state) wireOpened() map[int64]bool {
	open := map[int64]bool{}
	for _, raw := range st.query["open"] {
		if seq, err := strconv.ParseInt(raw, 10, 64); err == nil {
			open[seq] = true
		}
	}
	return open
}

func (st *state) wireUnfoldHref(seq int64) string {
	open := append(slices.Clone(st.query["open"]), itoa64(seq))
	return st.href(map[string][]string{"open": open})
}

func (st *state) wireModel(v api.WireView) wireModel {
	m := newWireModel(v.Calls, st.wireTiedOnly(), v.Scope == api.WireScopeFlow, st.wireOpened())
	m.titles = wireStepTitles(v)
	return m
}

func wireStepTitles(v api.WireView) map[string]string {
	titles := map[string]string{}
	for _, s := range v.Steps {
		if s.Flow == v.Flow && s.Title != "" {
			titles[s.Step] = s.Title
		}
	}
	return titles
}

func (st *state) wireLinks() wireLinks {
	picked, _ := strconv.ParseInt(st.query.Get("call"), 10, 64)
	return wireLinks{picked: picked, call: st.wireCallHref, unfold: st.wireUnfoldHref}
}

func (st *state) wireScene(v api.WireView) wireScene {
	if st.wireViewName() == wireViewTrace {
		return wireTrace(st.wireModel(v), st.wireLinks())
	}
	return wireSequence(st.wireModel(v), st.wireLinks())
}

func wireViewLabel(name string) string {
	switch name {
	case wireViewTrace:
		return "Trace"
	case wireViewCalls:
		return "Calls"
	}
	return "Sequence"
}

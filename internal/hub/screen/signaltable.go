package screen

import (
	"slices"
	"strconv"

	"github.com/d0lim/cockpit/internal/contract"
)

type signalTable struct {
	name    string
	columns []string
	rows    []contract.Signal
	total   int
}

var firstColumns = []string{"label", "status", "tone"}

func tables(signals []contract.Signal) []signalTable {
	var out []signalTable
	at := map[string]int{}
	for _, s := range signals {
		if s.Kind == contract.SignalKindSpan {
			continue
		}
		name := attrText(s.Attrs["table"])
		if name == "" {
			continue
		}
		i, ok := at[name]
		if !ok {
			i = len(out)
			at[name] = i
			out = append(out, signalTable{name: name})
		}
		out[i].rows = append(out[i].rows, s)
	}
	for i := range out {
		out[i].total = len(out[i].rows)
		out[i].rows = lastN(out[i].rows, tableWindow)
		out[i].columns = columnsOf(out[i].rows)
	}
	return out
}

func columnsOf(rows []contract.Signal) []string {
	keys := map[string]bool{}
	for _, s := range rows {
		for k := range s.Attrs {
			if k != "table" {
				keys[k] = true
			}
		}
		if s.Metric != nil {
			keys["value"] = true
		}
	}
	var out []string
	for _, k := range firstColumns {
		if keys[k] {
			out = append(out, k)
		}
	}
	var rest []string
	for k := range keys {
		if !slices.Contains(firstColumns, k) {
			rest = append(rest, k)
		}
	}
	slices.Sort(rest)
	return append(out, rest...)
}

func cellOf(s contract.Signal, column string) string {
	if column == "value" && s.Metric != nil {
		return strconv.FormatFloat(s.Metric.Value, 'f', -1, 64)
	}
	return attrText(s.Attrs[column])
}

type logSource struct {
	name  string
	lines []contract.Signal
}

func logSources(signals []contract.Signal) []logSource {
	var out []logSource
	at := map[string]int{}
	for _, s := range signals {
		if s.Collector != logCollector || s.Kind != contract.SignalKindEvent {
			continue
		}
		name := orElse(attrText(s.Attrs[containerAttr]), "(unknown)")
		i, ok := at[name]
		if !ok {
			i = len(out)
			at[name] = i
			out = append(out, logSource{name: name})
		}
		out[i].lines = append(out[i].lines, s)
	}
	return out
}

type serviceState struct{ status, health string }

func (st *state) serviceStates() map[string]serviceState {
	if st.services == nil {
		st.services = serviceStates(st.signals().signals)
	}
	return st.services
}

func serviceStates(signals []contract.Signal) map[string]serviceState {
	out := map[string]serviceState{}
	for _, s := range latestRevisions(signals) {
		if s.Kind != contract.SignalKindEvent || attrText(s.Attrs["table"]) != servicesTable {
			continue
		}
		if name := attrText(s.Attrs["label"]); name != "" {
			out[name] = serviceState{status: attrText(s.Attrs["status"]), health: attrText(s.Attrs["health"])}
		}
	}
	return out
}

func serviceTone(s serviceState) Tone {
	switch {
	case s.status == "running" && s.health == "starting":
		return ToneWarn
	case s.status == "running" && (s.health == "" || s.health == "healthy"):
		return ToneOK
	case s.status == "running":
		return ToneBad
	case s.status == "absent", s.status == "exited":
		return ToneIdle
	}
	return ToneBad
}

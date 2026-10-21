package screen

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
)

type signalTable struct {
	name    string
	columns []string
	rows    []contract.Signal
	bodies  []map[string]any
	total   int
	steps   bool
}

func (st *state) markSteps(list []signalTable) []signalTable {
	c := st.correlation()
	for i := range list {
		for _, s := range list[i].rows {
			if len(c.stepsOf(s)) > 0 {
				list[i].steps = true
				break
			}
		}
	}
	return list
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
		out[i].bodies = make([]map[string]any, len(out[i].rows))
		for j, s := range out[i].rows {
			out[i].bodies[j] = bodyObject(s)
		}
		out[i].columns = columnsOf(out[i].rows, out[i].bodies)
	}
	return out
}

func bodyObject(s contract.Signal) map[string]any {
	if s.Event == nil || !strings.HasPrefix(strings.TrimSpace(s.Event.Body), "{") {
		return nil
	}
	var m map[string]any
	if json.Unmarshal([]byte(s.Event.Body), &m) != nil {
		return nil
	}
	return m
}

func columnsOf(rows []contract.Signal, bodies []map[string]any) []string {
	keys := map[string]bool{}
	for _, s := range rows {
		for k := range s.Attrs {
			if k != "table" && !isRef(k) {
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
	out = append(out, rest...)
	seen := map[string]bool{}
	for i, s := range rows {
		if bodies[i] == nil {
			continue
		}
		for _, k := range bodyKeys(s) {
			if !keys[k] && !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

func bodyKeys(s contract.Signal) []string {
	if s.Event == nil {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(s.Event.Body))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil
	}
	var keys []string
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return keys
		}
		if k, ok := t.(string); ok {
			keys = append(keys, k)
		}
		var skip json.RawMessage
		if dec.Decode(&skip) != nil {
			return keys
		}
	}
	return keys
}

func cellOf(s contract.Signal, body map[string]any, column string) string {
	if column == "value" && s.Metric != nil {
		return strconv.FormatFloat(s.Metric.Value, 'f', -1, 64)
	}
	if v, ok := s.Attrs[column]; ok {
		return attrText(v)
	}
	return attrText(body[column])
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

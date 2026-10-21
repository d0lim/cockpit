package screen

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/signal"
)

const (
	maxRelated = 40
	stepPrefix = "step:"
)

type correlation struct {
	ids    map[string]bool
	carry  map[string][]contract.Signal
	values map[string][]string
	parent map[string]string
	groups map[string][]string
}

func (c *correlation) root(id string) string {
	for c.parent[id] != id {
		c.parent[id] = c.parent[c.parent[id]]
		id = c.parent[id]
	}
	return id
}

func (c *correlation) join(ids []string) {
	for _, id := range ids {
		if _, ok := c.parent[id]; !ok {
			c.parent[id] = id
		}
	}
	for _, id := range ids[min(1, len(ids)):] {
		if a, b := c.root(ids[0]), c.root(id); a != b {
			c.parent[b] = a
		}
	}
}

func (c *correlation) group(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if _, ok := c.parent[id]; !ok {
			continue
		}
		r := c.root(id)
		if seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, c.groups[r]...)
	}
	return out
}

func isRef(key string) bool { return strings.HasPrefix(key, signal.RefPrefix) }

func isStepSpan(s contract.Signal) bool {
	return s.Kind == contract.SignalKindSpan && s.Collector == signal.RunnerSource && strings.HasPrefix(s.ID, stepPrefix)
}

func valuesOf(s contract.Signal) []string {
	var out []string
	for _, v := range s.Attrs {
		if t := attrText(v); t != "" {
			out = append(out, t)
		}
	}
	if s.Event == nil || !strings.HasPrefix(strings.TrimSpace(s.Event.Body), "{") {
		return out
	}
	var body map[string]any
	if json.Unmarshal([]byte(s.Event.Body), &body) != nil {
		return out
	}
	for _, v := range body {
		switch v.(type) {
		case map[string]any, []any, nil:
			continue
		}
		if t := attrText(v); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func correlate(signals []contract.Signal) *correlation {
	c := &correlation{ids: map[string]bool{}, carry: map[string][]contract.Signal{}, values: map[string][]string{}, parent: map[string]string{}, groups: map[string][]string{}}
	for _, s := range signals {
		var declared []string
		for _, k := range sortedKeys(s.Attrs) {
			if isRef(k) {
				if t := attrText(s.Attrs[k]); t != "" && !strings.Contains(t, "****") {
					c.ids[t] = true
					declared = append(declared, t)
				}
			}
		}
		c.join(declared)
	}
	if len(c.ids) == 0 {
		return c
	}
	for _, id := range sortedSet(c.ids) {
		r := c.root(id)
		c.groups[r] = append(c.groups[r], id)
	}
	for _, s := range signals {
		key := sigKey(s)
		seen := map[string]bool{}
		for _, v := range valuesOf(s) {
			if c.ids[v] && !seen[v] {
				seen[v] = true
				c.values[key] = append(c.values[key], v)
				c.carry[v] = append(c.carry[v], s)
			}
		}
	}
	return c
}

func sigKey(s contract.Signal) string {
	if s.Span != nil {
		return "span\x00" + s.Span.TraceID + "\x00" + s.ID
	}
	return selectionKey(s)
}

func (c *correlation) idsOf(s contract.Signal) []string { return c.values[sigKey(s)] }

type relatedSet struct {
	steps  []contract.Signal
	others []contract.Signal
	more   int
}

func (c *correlation) related(ids []string, self string) relatedSet {
	var r relatedSet
	seen := map[string]bool{self: true}
	for _, id := range c.group(ids) {
		for _, s := range c.carry[id] {
			key := sigKey(s)
			if seen[key] {
				continue
			}
			seen[key] = true
			switch {
			case isStepSpan(s):
				r.steps = append(r.steps, s)
			case len(r.others) < maxRelated:
				r.others = append(r.others, s)
			default:
				r.more++
			}
		}
	}
	return r
}

func (c *correlation) stepsOf(s contract.Signal) []contract.Signal {
	return c.related(c.idsOf(s), sigKey(s)).steps
}

func (st *state) correlation() *correlation {
	if st.corr == nil {
		st.corr = correlate(latestRevisions(st.signals().signals))
	}
	return st.corr
}

func (st *state) signalHref(s contract.Signal) string {
	base := "/runs/" + st.run.ID
	switch {
	case isStepSpan(s):
		flow, step, _ := strings.Cut(strings.TrimPrefix(s.ID, stepPrefix), "/")
		return base + "/report/" + flow + "?step=" + url.QueryEscape(step)
	case s.Span != nil:
		return base + "/signals/trace?" + url.Values{"trace": {s.Span.TraceID}, "span": {s.ID}}.Encode()
	case s.Collector == logCollector:
		return base + "/signals/logs?" + url.Values{"source": {attrText(s.Attrs[containerAttr])}}.Encode()
	}
	return base + "/signals/events?" + url.Values{"event": {selectionKey(s)}}.Encode()
}

func (st *state) searchHref(value string) string {
	return "/runs/" + st.run.ID + "/signals/events?" + url.Values{"q": {value}}.Encode()
}

func signalName(s contract.Signal) string {
	switch {
	case isStepSpan(s):
		return strings.TrimPrefix(s.ID, stepPrefix)
	case s.Span != nil:
		return s.Span.Name
	}
	if t := attrText(s.Attrs["table"]); t != "" {
		return t + " · " + orElse(s.ID, eventTitle(s))
	}
	return s.Collector + " · " + eventTitle(s)
}

func refAttrs(s contract.Signal) []chip {
	var out []chip
	for _, k := range sortedKeys(s.Attrs) {
		if isRef(k) {
			out = append(out, chip{strings.TrimPrefix(k, signal.RefPrefix), attrText(s.Attrs[k])})
		}
	}
	return out
}

func unnamed(s contract.Signal, ids []string) []string {
	named := map[string]bool{}
	for _, r := range refAttrs(s) {
		named[r.value] = true
	}
	var out []string
	for _, v := range ids {
		if !named[v] {
			out = append(out, v)
		}
	}
	return out
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

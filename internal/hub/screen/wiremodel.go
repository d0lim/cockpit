package screen

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/wire"
)

const (
	wireViewSequence = "sequence"
	wireViewTrace    = "trace"
	wireViewCalls    = "calls"
	wireTiesOnly     = "only"
	wireFoldAt       = 3
)

var (
	wireDigits    = regexp.MustCompile(`[0-9]+`)
	wireRunner    = "cockpit-runner"
	wireViewNames = []string{wireViewSequence, wireViewTrace, wireViewCalls}
)

type wireNode struct {
	head  api.WireHead
	start float64
	reply float64
	end   float64
	fold  []*wireNode
	kids  []*wireNode
	level int
}

type wireModel struct {
	nodes []*wireNode
	shown map[int64]*wireNode
	flow  bool
}

func wireMillis(n *wireNode) float64 { return n.end - n.start }

func newWireModel(calls []api.WireHead, tiedOnly, flow bool, open map[int64]bool) wireModel {
	m := wireModel{shown: map[int64]*wireNode{}, flow: flow}
	if len(calls) == 0 {
		return m
	}
	base := calls[0].Started
	for _, c := range calls {
		if c.Started.Before(base) {
			base = c.Started
		}
	}
	var kept []*wireNode
	bySeq := map[int64]*wireNode{}
	for _, c := range calls {
		if tiedOnly && !api.WireLinkTied(c.Link) {
			continue
		}
		n := &wireNode{head: c, start: float64(c.Started.Sub(base)) / 1e6}
		n.end = n.start + float64(c.DurationMs)
		if !c.Finished.IsZero() {
			n.end = float64(c.Finished.Sub(base)) / 1e6
		}
		n.reply = n.end
		if !c.Replied.IsZero() {
			n.reply = max(float64(c.Replied.Sub(base))/1e6, n.start)
		}
		kept, bySeq[c.Seq] = append(kept, n), n
	}
	var roots []*wireNode
	for _, n := range kept {
		if parent := bySeq[n.head.Parent]; parent != nil && parent != n {
			parent.kids = append(parent.kids, n)
		} else {
			roots = append(roots, n)
		}
	}
	m.collect(roots, open)
	slices.SortStableFunc(m.nodes, func(a, b *wireNode) int {
		return cmp.Or(cmp.Compare(a.start, b.start), cmp.Compare(a.head.Seq, b.head.Seq))
	})
	return m
}

func (m *wireModel) collect(list []*wireNode, open map[int64]bool) {
	groups := map[string][]*wireNode{}
	var order []string
	for _, n := range list {
		key := wireShape(n)
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], n)
	}
	for _, key := range order {
		group := groups[key]
		if len(group) >= wireFoldAt && !open[group[0].head.Seq] {
			group[0].fold = group
			group = group[:1]
		}
		for _, n := range group {
			m.nodes = append(m.nodes, n)
			m.shown[n.head.Seq] = n
			m.collect(n.kids, open)
		}
	}
}

func wireShape(n *wireNode) string {
	h := n.head
	kids := make([]string, 0, len(n.kids))
	for _, k := range n.kids {
		kids = append(kids, wireShape(k))
	}
	slices.Sort(kids)
	return strings.Join([]string{h.Caller, h.Target, h.Method, wirePathShape(h.Path), itoa(h.Status), string(h.Link), h.Step}, "\x00") + "[" + strings.Join(kids, ",") + "]"
}

func wirePathShape(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if len(part) >= 12 && strings.ContainsAny(part, "0123456789") {
			parts[i] = "#"
		}
	}
	return wireDigits.ReplaceAllString(strings.Join(parts, "/"), "#")
}

func wireFoldEvery(fold []*wireNode) float64 {
	gaps := make([]float64, 0, len(fold)-1)
	for i := 1; i < len(fold); i++ {
		gaps = append(gaps, fold[i].start-fold[i-1].start)
	}
	slices.Sort(gaps)
	return gaps[len(gaps)/2]
}

func (m wireModel) lanes() []string {
	var out []string
	add := func(name string) {
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	for _, n := range m.nodes {
		if n.head.Caller == wireRunner {
			add(wireRunner)
			break
		}
	}
	for _, n := range m.nodes {
		add(n.head.Caller)
		add(n.head.Target)
	}
	return out
}

func wireLaneName(name string) string {
	if name == wireRunner {
		return "runner"
	}
	return name
}

func wireMs(ms float64) string {
	switch {
	case ms < 1:
		return strconv.FormatFloat(ms, 'f', 1, 64) + "ms"
	case ms < 1000:
		return strconv.FormatFloat(ms, 'f', 0, 64) + "ms"
	case ms < 10000:
		return strconv.FormatFloat(ms/1000, 'f', 2, 64) + "s"
	}
	return strconv.FormatFloat(ms/1000, 'f', 1, 64) + "s"
}

func wireEvidence(h api.WireHead) string {
	class := "wire-ev-" + string(h.Link)
	if h.Status == 0 || h.Status >= 400 {
		class += " wire-failed"
	}
	return class
}

func wireHollow(h api.WireHead) bool {
	return h.Link == wire.LinkTime || h.Link == wire.LinkBackground || h.Link == wire.LinkNone
}

func wireStatusText(h api.WireHead) string {
	if h.Status == 0 {
		return wireOutcomes[h.Outcome]
	}
	return itoa(h.Status)
}

func wireShortPath(path string, room int) string {
	if path == "" {
		return "(path not kept)"
	}
	runes := []rune(path)
	if len(runes) <= room {
		return path
	}
	parts := strings.Split(path, "/")
	tail := ""
	for i := len(parts) - 1; i > 0; i-- {
		next := "/" + parts[i] + tail
		if len([]rune(next))+1 > room {
			break
		}
		tail = next
	}
	if tail != "" {
		return "…" + tail
	}
	return "…" + string(runes[len(runes)-max(room-1, 1):])
}

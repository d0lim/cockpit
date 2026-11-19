package screen

import (
	"fmt"
	"slices"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/wire"
)

const (
	traceWidth  = 832.0
	traceLabels = 292.0
	traceTime   = 50.0
	traceRow    = 30.0
	traceIndent = 14.0
	traceBreak  = 13.0
	traceHead   = 22.0
	traceGroup  = 20.0
	traceIdleMs = 100.0
	traceShare  = 0.05
)

type traceSegment struct{ from, to float64 }

type traceAxis struct {
	left     float64
	perMs    float64
	segments []traceSegment
	right    float64
}

type traceDraw struct {
	scene wireScene
	links wireLinks
	model wireModel
	axis  traceAxis
	y     float64
}

var traceGroups = []string{"Started by the step", "Continues an earlier call", "Callback chains", "Background work", "Same time only"}

func wireTrace(m wireModel, links wireLinks) wireScene {
	d := traceDraw{links: links, model: m, axis: newTraceAxis(m), y: 2}
	d.scene = wireScene{Label: "Trace of the calls", Width: traceWidth, HeadHeight: traceHead}
	d.drawAxis()
	groups := make([][]*wireNode, len(traceGroups))
	for _, n := range m.nodes {
		if m.shown[n.head.Parent] == nil || n.head.Parent == 0 {
			at := traceGroupOf(m, n)
			groups[at] = append(groups[at], n)
		}
	}
	for i, roots := range groups {
		if len(roots) == 0 {
			continue
		}
		d.text(wireText{X: 6, Y: d.y + 14, Text: traceGroups[i], Class: "wire-group wire-small"})
		d.y += traceGroup
		for _, n := range roots {
			d.row(n, 0)
		}
	}
	d.scene.Height = d.y + 6
	return d.scene
}

func traceGroupOf(m wireModel, n *wireNode) int {
	h := n.head
	switch {
	case h.Link == wire.LinkExact:
		return 0
	case h.Parent != 0:
		return 1
	case h.Link == wire.LinkBackground:
		return 3
	case traceHasShownKid(m, n):
		return 2
	}
	return 4
}

func traceHasShownKid(m wireModel, n *wireNode) bool {
	return slices.ContainsFunc(n.kids, func(k *wireNode) bool { return m.shown[k.head.Seq] != nil })
}

func newTraceAxis(m wireModel) traceAxis {
	var spans []traceSegment
	last := 0.0
	for _, n := range m.nodes {
		for _, f := range traceRuns(n) {
			spans = append(spans, traceSegment{f.start, max(f.end, f.start+0.5)})
			last = max(last, f.end)
		}
	}
	slices.SortFunc(spans, func(a, b traceSegment) int {
		switch {
		case a.from < b.from:
			return -1
		case a.from > b.from:
			return 1
		}
		return 0
	})
	limit := max(traceIdleMs, last*traceShare)
	var merged []traceSegment
	for _, s := range spans {
		if at := len(merged) - 1; at >= 0 && s.from-merged[at].to < limit {
			merged[at].to = max(merged[at].to, s.to)
			continue
		}
		merged = append(merged, s)
	}
	live := 0.0
	for _, s := range merged {
		live += s.to - s.from
	}
	left, right := traceLabels+8, traceWidth-traceTime
	breaks := traceBreak * float64(max(len(merged)-1, 0))
	return traceAxis{left: left, right: right, segments: merged, perMs: (right - left - breaks) / max(live, 1)}
}

func traceRuns(n *wireNode) []*wireNode {
	if n.fold != nil {
		return n.fold
	}
	return []*wireNode{n}
}

func (a traceAxis) x(ms float64) float64 {
	x := a.left
	for _, s := range a.segments {
		if ms <= s.to {
			return x + max(ms-s.from, 0)*a.perMs
		}
		x += (s.to-s.from)*a.perMs + traceBreak
	}
	return a.right
}

func (d *traceDraw) text(t wireText) { d.scene.Texts = append(d.scene.Texts, t) }

func (d *traceDraw) drawAxis() {
	s := &d.scene
	end := 0.0
	for _, n := range d.model.nodes {
		for _, f := range traceRuns(n) {
			end = max(end, f.end)
		}
	}
	s.Head = append(s.Head,
		wireText{X: 6, Y: 14, Text: "call", Class: "wire-dim wire-small"},
		wireText{X: d.axis.left, Y: 14, Text: "0", Class: "wire-dim wire-small"},
		wireText{X: traceWidth - 6, Y: 14, Text: wireMs(end), Class: "wire-dim wire-small", Anchor: "end"})
	for i, seg := range d.axis.segments {
		if i > 0 {
			x := d.axis.x(seg.from)
			s.HeadPaths = append(s.HeadPaths, wirePath{D: fmt.Sprintf("M%s 17l3 -6M%s 17l3 -6", wireNum(x-9), wireNum(x-5)), Class: "wire-tree"})
		}
	}
}

func (d *traceDraw) row(n *wireNode, depth int) {
	h := n.head
	x := 8 + float64(depth)*traceIndent
	d.scene.hit(d.links, h, d.y, traceRow)
	if depth > 0 {
		d.scene.Lines = append(d.scene.Lines, wirePath{D: fmt.Sprintf("M%s %sV%sH%s", wireNum(x-9), wireNum(d.y-14), wireNum(d.y+11), wireNum(x-3)), Class: "wire-tree"})
	}
	d.labels(n, x)
	if n.fold != nil {
		d.foldBars(n)
	} else {
		d.bar(n)
	}
	d.y += traceRow
	for _, kid := range n.kids {
		if d.model.shown[kid.head.Seq] != nil {
			d.row(kid, depth+1)
		}
	}
}

func (d *traceDraw) labels(n *wireNode, x float64) {
	h := n.head
	spare := 8.0
	if n.fold != nil {
		spare = 36
	}
	tag := ""
	if h.Parent != 0 && d.model.shown[h.Parent] == nil {
		tag = "↖#" + itoa64(h.Parent) + " "
	}
	room := max(int((traceLabels-x-spare)/wireCharWidth), 14) - len([]rune(h.Method)) - 1 - len([]rune(tag))
	d.text(wireText{X: x, Y: d.y + 12, Tag: tag, Text: h.Method + " " + wireShortPath(h.Path, room)})
	route := wireLaneName(h.Caller) + " → " + h.Target
	if h.Root != "" {
		route += " · " + h.Root
	}
	d.text(wireText{X: x, Y: d.y + 24, Text: wireCut(route, int((traceLabels-x-8)/5.8)), Class: "wire-dim wire-small"})
	if n.fold != nil {
		d.text(wireText{X: traceLabels - 6, Y: d.y + 12, Text: "×" + itoa(len(n.fold)), Class: "wire-fold", Anchor: "end", Href: d.links.unfold(h.Seq)})
	}
}

func (d *traceDraw) foldBars(n *wireNode) {
	longest := 0.0
	for _, f := range n.fold {
		longest = max(longest, wireMillis(f))
		d.scene.Bars = append(d.scene.Bars, wireRect{X: d.axis.x(f.start), Y: d.y + 7, W: 2.5, H: 16, Class: "wire-bar " + wireEvidence(f.head)})
	}
	d.duration(n.head, longest)
}

func (d *traceDraw) bar(n *wireNode) {
	h := n.head
	from, answered, end := d.axis.x(n.start), d.axis.x(n.reply), d.axis.x(n.end)
	class := wireEvidence(h)
	d.scene.Bars = append(d.scene.Bars,
		wireRect{X: from, Y: d.y + 9, W: max(end-from, 2.5), H: 12, Class: "wire-bar wire-bar-tail " + class},
		wireRect{X: from, Y: d.y + 9, W: max(answered-from, 2.5), H: 12, Class: "wire-bar " + class})
	d.duration(h, wireMillis(n))
}

func (d *traceDraw) duration(h api.WireHead, ms float64) {
	text := wireMs(ms)
	if h.Status == 0 || h.Status >= 400 {
		text = wireStatusText(h) + " " + text
	}
	d.text(wireText{X: traceWidth - 6, Y: d.y + 18, Text: text, Class: wireResultClass(h), Anchor: "end"})
}

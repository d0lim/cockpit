package screen

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/hub/api"
)

const (
	seqGutter  = 46.0
	seqLane    = 104.0
	seqHead    = 36.0
	seqPairRow = 32.0
	seqCallRow = 24.0
	seqSelfRow = 30.0
	seqReply   = 16.0
	seqGapRow  = 14.0
	seqBandRow = 22.0
	seqGapMs   = 150.0
	seqBarHalf = 3.5
	seqBarStep = 4.0
	seqLaneCap = 13
	seqGlyph   = 6.5
	seqWide    = 11.5
	seqWideMin = 0x1100
)

type seqEvent struct {
	node  *wireNode
	reply bool
	at    float64
}

type seqDraw struct {
	scene   wireScene
	links   wireLinks
	lane    map[string]float64
	active  map[string]int
	asked   map[int64]float64
	replied map[int64]float64
	y       float64
}

func wireSequence(m wireModel, links wireLinks) wireScene {
	lanes := m.lanes()
	d := seqDraw{links: links, lane: map[string]float64{}, active: map[string]int{}, asked: map[int64]float64{}, replied: map[int64]float64{}, y: 6}
	d.scene = wireScene{Label: "Sequence of the calls", Width: seqGutter + seqLane*float64(len(lanes)), HeadHeight: seqHead}
	for i, name := range lanes {
		d.lane[name] = seqGutter + seqLane*(float64(i)+0.5)
		d.headName(name)
	}
	events := seqEvents(m)
	band, last, seen := "", 0.0, false
	for i := 0; i < len(events); i++ {
		e := events[i]
		if !e.reply && m.flow && e.node.head.Step != band {
			band = e.node.head.Step
			d.band(wireStepLabel(m.titles, band))
			seen = false
		}
		if seen && e.at-last >= seqGapMs {
			d.gap(e.at - last)
		}
		last, seen = e.at, true
		if e.reply {
			d.lateReply(e.node)
			continue
		}
		paired := i+1 < len(events) && events[i+1].node == e.node && events[i+1].reply
		d.request(m, e.node, paired)
		if paired {
			i++
		}
	}
	d.finish(m, lanes)
	return d.scene
}

func seqEvents(m wireModel) []seqEvent {
	events := make([]seqEvent, 0, len(m.nodes)*2)
	for _, n := range m.nodes {
		events = append(events, seqEvent{node: n, at: n.start}, seqEvent{node: n, reply: true, at: n.reply})
	}
	slices.SortStableFunc(events, func(a, b seqEvent) int {
		if c := cmp.Compare(a.at, b.at); c != 0 {
			return c
		}
		switch {
		case a.reply && !b.reply:
			return 1
		case !a.reply && b.reply:
			return -1
		case a.reply:
			return cmp.Compare(b.node.head.Seq, a.node.head.Seq)
		}
		return cmp.Compare(a.node.head.Seq, b.node.head.Seq)
	})
	return events
}

func (d *seqDraw) headName(name string) {
	x, text := d.lane[name], wireLaneName(name)
	lines := []string{text}
	if cut := strings.LastIndex(text[:min(len(text), seqLaneCap)], "-"); len(text) > seqLaneCap && cut > 0 {
		lines = []string{text[:cut+1], wireCut(text[cut+1:], seqLaneCap)}
	} else if len(text) > seqLaneCap {
		lines = []string{wireCut(text, seqLaneCap)}
	}
	y := 21.0
	if len(lines) == 2 {
		y = 14
	}
	for i, line := range lines {
		d.scene.Head = append(d.scene.Head, wireText{X: x, Y: y + float64(i)*12, Text: line, Class: "wire-name", Anchor: "middle"})
	}
}

func wireFit(text string, width float64) string {
	used, room := 0.0, 0
	for _, r := range text {
		w := seqGlyph
		if r >= seqWideMin {
			w = seqWide
		}
		if used+w > width {
			return wireCut(text, max(room, 2))
		}
		used += w
		room++
	}
	return text
}

func wireCut(text string, room int) string {
	runes := []rune(text)
	if len(runes) <= room {
		return text
	}
	return string(runes[:room-1]) + "…"
}

func (d *seqDraw) band(label string) {
	d.scene.Bands = append(d.scene.Bands, wireRect{Y: d.y, W: d.scene.Width, H: 18, Class: "wire-band"})
	d.text(wireText{X: 6, Y: d.y + 13, Text: wireFit(label, d.scene.Width-12), Class: "wire-name"})
	d.y += seqBandRow
}

func (d *seqDraw) gap(ms float64) {
	d.scene.Lines = append(d.scene.Lines, wirePath{D: fmt.Sprintf("M0 %sH%s", wireNum(d.y+6), wireNum(d.scene.Width)), Class: "wire-gap"})
	d.text(wireText{X: d.scene.Width - 6, Y: d.y + 9, Text: wireMs(ms) + " later", Class: "wire-dim wire-small wire-halo", Anchor: "end"})
	d.y += seqGapRow
}

func (d *seqDraw) text(t wireText) { d.scene.Texts = append(d.scene.Texts, t) }

func (d *seqDraw) request(m wireModel, n *wireNode, paired bool) {
	h := n.head
	from, to := d.lane[h.Caller], d.lane[h.Target]
	self := h.Caller == h.Target
	dir := 1.0
	if to < from {
		dir = -1
	}
	callerLevel, targetLevel := d.active[h.Caller], d.active[h.Target]
	n.level = targetLevel
	d.active[h.Target] = targetLevel + 1
	y := d.y + 16
	d.asked[h.Seq] = y
	height := seqCallRow
	switch {
	case self:
		height = seqSelfRow
	case paired:
		height = seqPairRow
	}
	d.scene.hit(d.links, h, d.y, height)
	d.text(wireText{X: 4, Y: y + 3, Text: "+" + wireMs(n.start), Class: "wire-dim wire-small"})
	d.label(m, n, min(from, to)+6, d.y+10)
	class := wireEvidence(h)
	if self {
		d.scene.Paths = append(d.scene.Paths, wirePath{D: fmt.Sprintf("M%s %sh20v9h-16", wireNum(from+4), wireNum(y)), Class: "wire-line " + class})
		d.tip(from+4, y+9, -1, class, wireHollow(h))
		d.text(wireText{X: from + 28, Y: y + 8, Text: wireStatusText(h) + " · " + wireMs(wireMillis(n)), Class: wireResultClass(h)})
		d.replied[h.Seq] = y + 9
		d.active[h.Target] = targetLevel
	} else {
		start := from
		if callerLevel > 0 {
			start = from + dir*seqBarHalf + float64(callerLevel-1)*seqBarStep
		}
		tip := to - dir*seqBarHalf + float64(targetLevel)*seqBarStep
		d.scene.Paths = append(d.scene.Paths, wirePath{D: fmt.Sprintf("M%s %sL%s %s", wireNum(start), wireNum(y), wireNum(tip-dir*5), wireNum(y)), Class: "wire-line " + class})
		d.tip(tip, y, dir, class, wireHollow(h))
	}
	d.cause(m, n, from, y)
	d.y += height
	if paired && !self {
		d.active[h.Target] = targetLevel
		d.reply(n, y+11)
	}
}

func (d *seqDraw) tip(x, y, dir float64, class string, hollow bool) {
	if hollow {
		class += " wire-open"
	}
	d.scene.Paths = append(d.scene.Paths, wirePath{D: fmt.Sprintf("M%s %sl%s -3.4v6.8z", wireNum(x), wireNum(y), wireNum(-dir*7)), Class: "wire-tip " + class})
}

func (d *seqDraw) label(m wireModel, n *wireNode, x, y float64) {
	h := n.head
	spare := 6.0
	if n.fold != nil {
		spare = 104
	}
	tag := ""
	if _, inside := m.shown[h.Parent]; h.Parent != 0 && !inside {
		tag = "↖#" + itoa64(h.Parent) + " "
	}
	room := max(int((d.scene.Width-x-spare)/wireCharWidth), 14) - len([]rune(h.Method)) - 1 - len([]rune(tag))
	d.text(wireText{X: x, Y: y, Tag: tag, Text: h.Method + " " + wireShortPath(h.Path, room), Class: "wire-halo"})
	if n.fold != nil {
		d.text(wireText{X: d.scene.Width - 6, Y: y, Text: "×" + itoa(len(n.fold)), Note: " every " + wireMs(wireFoldEvery(n.fold)), Class: "wire-fold wire-halo", Anchor: "end", Href: d.links.unfold(h.Seq)})
	}
}

func (d *seqDraw) cause(m wireModel, n *wireNode, x, y float64) {
	parent := m.shown[n.head.Parent]
	if parent == nil || n.head.Link != "traced" || parent.reply >= n.start || parent.head.Target != n.head.Caller {
		return
	}
	d.scene.Lines = append(d.scene.Lines, wirePath{D: fmt.Sprintf("M%s %sV%s", wireNum(x), wireNum(d.asked[parent.head.Seq]), wireNum(y)), Class: "wire-cause"})
}

func (d *seqDraw) lateReply(n *wireNode) {
	h := n.head
	if h.Caller == h.Target {
		return
	}
	d.active[h.Target] = max(d.active[h.Target]-1, 0)
	d.scene.hit(d.links, h, d.y, seqReply)
	d.reply(n, d.y+9)
	d.y += seqReply
}

func (d *seqDraw) reply(n *wireNode, y float64) {
	h := n.head
	from, to := d.lane[h.Target], d.lane[h.Caller]
	dir := 1.0
	if to < from {
		dir = -1
	}
	class := wireEvidence(h)
	d.replied[h.Seq] = y
	start := from + dir*seqBarHalf + float64(n.level)*seqBarStep
	d.scene.Paths = append(d.scene.Paths,
		wirePath{D: fmt.Sprintf("M%s %sL%s %s", wireNum(start), wireNum(y), wireNum(to), wireNum(y)), Class: "wire-reply " + class},
		wirePath{D: fmt.Sprintf("M%s %sL%s %sL%s %s", wireNum(to-dir*5), wireNum(y-3), wireNum(to), wireNum(y), wireNum(to-dir*5), wireNum(y+3)), Class: "wire-line " + class})
	right := max(from, to)
	t := wireText{X: right + 7, Y: y + 3, Text: wireStatusText(h) + " · " + wireMs(wireMillis(n)), Class: wireResultClass(h)}
	if right+74 > d.scene.Width {
		t.X, t.Y, t.Anchor = right-7, y-2, "end"
	}
	d.text(t)
}

func wireResultClass(h api.WireHead) string {
	if h.Status == 0 || h.Status >= 400 {
		return "wire-bad wire-small wire-halo"
	}
	return "wire-dim wire-small wire-halo"
}

func (d *seqDraw) finish(m wireModel, lanes []string) {
	s := &d.scene
	s.Height = d.y + 8
	for _, name := range lanes {
		x := d.lane[name]
		s.Lines = append([]wirePath{{D: fmt.Sprintf("M%s 0V%s", wireNum(x), wireNum(s.Height)), Class: "wire-lifeline"}}, s.Lines...)
		s.HeadPaths = append(s.HeadPaths, wirePath{D: fmt.Sprintf("M%s %sV%s", wireNum(x), wireNum(seqHead-6), wireNum(seqHead)), Class: "wire-lifeline"})
	}
	for _, n := range m.nodes {
		h := n.head
		asked, replied := d.asked[h.Seq], d.replied[h.Seq]
		if h.Caller != h.Target && replied-asked > 13 {
			s.Bars = append(s.Bars, wireRect{X: d.lane[h.Target] - seqBarHalf + float64(n.level)*seqBarStep, Y: asked, W: seqBarHalf * 2, H: replied - asked, Class: "wire-active"})
		}
	}
}

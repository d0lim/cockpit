package screen

import (
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/hub/api"
)

var wireMermaidText = strings.NewReplacer("#", "#35;", ";", "#59;", "%", "#37;", "\r", " ", "\n", " ")

func wireMermaid(m wireModel) string {
	var b strings.Builder
	b.WriteString("sequenceDiagram\n")
	lanes := m.lanes()
	alias := make(map[string]string, len(lanes))
	for i, name := range lanes {
		alias[name] = "p" + strconv.Itoa(i)
		b.WriteString("    participant " + alias[name] + " as " + wireMermaidText.Replace(wireLaneName(name)) + "\n")
	}
	span := ""
	if len(lanes) > 1 {
		span = alias[lanes[0]] + "," + alias[lanes[len(lanes)-1]]
	} else if len(lanes) == 1 {
		span = alias[lanes[0]]
	}
	step := ""
	for _, e := range seqEvents(m) {
		n, h := e.node, e.node.head
		if !e.reply && m.flow && h.Step != step {
			step = h.Step
			b.WriteString("    Note over " + span + ": step " + wireMermaidText.Replace(step) + "\n")
		}
		from, to := alias[h.Caller], alias[h.Target]
		if e.reply {
			b.WriteString("    " + to + wireMermaidReply(h) + from + ": " + wireMermaidText.Replace(wireStatusText(h)+" · "+wireMs(wireMillis(n))) + "\n")
			continue
		}
		b.WriteString("    " + from + wireMermaidArrow(h) + to + ": " + wireMermaidText.Replace(wireMermaidLabel(n)) + "\n")
	}
	return b.String()
}

func wireMermaidArrow(h api.WireHead) string {
	if wireHollow(h) {
		return "-)"
	}
	return "->>"
}

func wireMermaidReply(h api.WireHead) string {
	if h.Status == 0 {
		return "--x"
	}
	return "-->>"
}

func wireMermaidLabel(n *wireNode) string {
	h := n.head
	label := h.Method + " " + orElse(h.Path, "(path not kept)")
	if n.fold != nil {
		label += " ×" + itoa(len(n.fold)) + " every " + wireMs(wireFoldEvery(n.fold))
	}
	return label
}

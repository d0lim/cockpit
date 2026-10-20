package screen

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
)

type traceSummary struct {
	id       string
	root     string
	start    string
	duration *int64
	status   contract.SpanStatus
	spans    int
	exec     bool
}

type spanRow struct {
	s          contract.Signal
	id         string
	depth      int
	duration   *int64
	offset     float64
	width      float64
	marker     bool
	sourceMark string
}

var spanRank = map[contract.SpanStatus]int{contract.SpanStatusOK: 0, contract.SpanStatusIncomplete: 1, contract.SpanStatusError: 2}

var sourceMarks = map[contract.TimeSource]string{contract.TimeSourceHost: "H", contract.TimeSourceContainer: "C", contract.TimeSourceRunner: "R"}

func millis(iso string) (int64, bool) {
	t, err := time.Parse(time.RFC3339Nano, iso)
	if err != nil || iso == "" {
		return 0, false
	}
	return t.UnixMilli(), true
}

func spanID(s contract.Signal) string {
	if s.ID != "" {
		return s.ID
	}
	return "seq-" + strconv.FormatInt(s.Seq, 10)
}

func durationOf(b *contract.SpanBody) *int64 {
	a, ok1 := millis(b.Start)
	z, ok2 := millis(b.End)
	if !ok1 || !ok2 {
		return nil
	}
	d := max(0, z-a)
	return &d
}

func spansByTrace(signals []contract.Signal) (map[string][]contract.Signal, []string) {
	out := map[string][]contract.Signal{}
	var order []string
	for _, s := range signals {
		if s.Kind != contract.SignalKindSpan || s.Span == nil {
			continue
		}
		if _, ok := out[s.Span.TraceID]; !ok {
			order = append(order, s.Span.TraceID)
		}
		out[s.Span.TraceID] = append(out[s.Span.TraceID], s)
	}
	return out, order
}

func roots(spans []contract.Signal) []contract.Signal {
	ids := map[string]bool{}
	for _, s := range spans {
		ids[spanID(s)] = true
	}
	var out []contract.Signal
	for _, s := range spans {
		if s.Span.ParentID == "" || !ids[s.Span.ParentID] {
			out = append(out, s)
		}
	}
	return out
}

func traceList(signals []contract.Signal) []traceSummary {
	byTrace, order := spansByTrace(signals)
	var out []traceSummary
	for _, id := range order {
		spans := byTrace[id]
		t := traceSummary{id: id, root: id, status: contract.SpanStatusOK, spans: len(spans), exec: strings.Contains(id, "/exec/")}
		var first, last int64
		var hasFirst, hasLast bool
		for _, s := range spans {
			if v, ok := millis(s.Span.Start); ok && (!hasFirst || v < first) {
				first, hasFirst = v, true
			}
			if v, ok := millis(s.Span.End); ok && (!hasLast || v > last) {
				last, hasLast = v, true
			}
			if spanRank[s.Span.Status] > spanRank[t.status] {
				t.status = s.Span.Status
			}
		}
		if r := roots(spans); len(r) > 0 {
			t.root = r[0].Span.Name
		}
		if hasFirst {
			t.start = time.UnixMilli(first).UTC().Format("2006-01-02T15:04:05.000Z")
		}
		if hasFirst && hasLast {
			d := max(0, last-first)
			t.duration = &d
		}
		out = append(out, t)
	}
	slices.SortStableFunc(out, func(a, b traceSummary) int {
		if a.exec != b.exec {
			if a.exec {
				return -1
			}
			return 1
		}
		if c := strings.Compare(b.start, a.start); c != 0 {
			return c
		}
		return strings.Compare(a.id, b.id)
	})
	return out
}

func spanRows(spans []contract.Signal) []spanRow {
	startOf := func(s contract.Signal) int64 { v, _ := millis(s.Span.Start); return v }
	byStart := func(a, b contract.Signal) int {
		if d := startOf(a) - startOf(b); d != 0 {
			return int(max(-1, min(1, d)))
		}
		return int(a.Seq - b.Seq)
	}
	children := map[string][]contract.Signal{}
	for _, s := range spans {
		if s.Span.ParentID != "" {
			children[s.Span.ParentID] = append(children[s.Span.ParentID], s)
		}
	}
	top := roots(spans)
	slices.SortStableFunc(top, byStart)
	var t0, tEnd int64
	first := true
	for _, s := range spans {
		a, ok := millis(s.Span.Start)
		if !ok {
			continue
		}
		z, ok := millis(s.Span.End)
		if !ok {
			z = a
		}
		if first || a < t0 {
			t0 = a
		}
		if first || z > tEnd {
			tEnd = z
		}
		first = false
	}
	total := float64(max(1, tEnd-t0))
	var rootSource contract.TimeSource
	if len(top) > 0 {
		rootSource = top[0].TSSource
	}
	var rows []spanRow
	seen := map[string]bool{}
	var visit func(s contract.Signal, depth int)
	visit = func(s contract.Signal, depth int) {
		id := spanID(s)
		if seen[id] {
			return
		}
		seen[id] = true
		d := durationOf(s.Span)
		row := spanRow{s: s, id: id, depth: depth, duration: d, offset: float64(startOf(s)-t0) / total * 100, marker: d == nil || *d == 0}
		if _, ok := millis(s.Span.Start); !ok {
			row.offset = 0
		}
		if d != nil {
			row.width = float64(*d) / total * 100
		}
		row.width = max(row.width, 0.5)
		if rootSource != "" && s.TSSource != rootSource {
			row.sourceMark = sourceMarks[s.TSSource]
		}
		rows = append(rows, row)
		kids := children[id]
		slices.SortStableFunc(kids, byStart)
		for _, c := range kids {
			visit(c, depth+1)
		}
	}
	for _, r := range top {
		visit(r, 0)
	}
	return rows
}

func durationText(d *int64) string {
	switch {
	case d == nil:
		return "no end"
	case *d < 1000:
		return strconv.FormatInt(*d, 10) + "ms"
	}
	return strconv.FormatFloat(float64(*d)/1000, 'f', 1, 64) + "s"
}

func spanTone(s contract.SpanStatus) Tone {
	switch s {
	case contract.SpanStatusOK:
		return ToneOK
	case contract.SpanStatusError:
		return ToneBad
	}
	return ToneWarn
}

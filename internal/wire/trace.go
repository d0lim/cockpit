package wire

import (
	"slices"
	"strings"
	"time"
)

const (
	rootNameLimit = 120
	routeClockGap = 2 * time.Millisecond
)

var (
	methodAttrs = []string{"http.request.method", "http.method", "method"}
	routeAttrs  = []string{"http.route", "url.path", "http.target", "uri"}
	httpMethods = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE", "CONNECT"}
)

type Span struct {
	ID      string
	Parent  string
	Service string
	Name    string
	Method  string
	Route   string
	Start   time.Time
}

func NewSpan(id, parent, service, name string, attrs map[string]any, start time.Time) Span {
	span := Span{ID: id, Parent: parent, Service: service, Name: name, Start: start}
	span.Method, span.Route = firstAttr(attrs, methodAttrs), firstAttr(attrs, routeAttrs)
	for _, word := range strings.Fields(name) {
		upper := strings.ToUpper(word)
		switch {
		case span.Method == "" && slices.Contains(httpMethods, upper):
			span.Method = upper
		case span.Route == "" && strings.HasPrefix(word, "/"):
			span.Route = word
		}
	}
	span.Route, _, _ = strings.Cut(span.Route, "?")
	return span
}

func firstAttr(attrs map[string]any, names []string) string {
	for _, name := range names {
		if text, ok := attrs[name].(string); ok && text != "" {
			return text
		}
	}
	return ""
}

type chainEnd struct {
	parent int
	found  bool
	route  bool
	rooted bool
	root   string
}

type spanChains struct {
	calls  []Call
	spans  map[string]Span
	callOf map[string]int
	into   map[string][]int
}

func newSpanChains(calls []Call, order []int, spans []Span) spanChains {
	g := spanChains{calls: calls, spans: make(map[string]Span, len(spans)), callOf: map[string]int{}, into: map[string][]int{}}
	for _, s := range spans {
		g.spans[s.ID] = s
	}
	for _, i := range order {
		c := calls[i]
		g.into[c.Target] = append(g.into[c.Target], i)
		if _, taken := g.callOf[c.Span]; c.Span != "" && !taken {
			g.callOf[c.Span] = i
		}
	}
	return g
}

func (g spanChains) reach(i int) chainEnd {
	id := g.calls[i].Span
	span, ok := g.spans[id]
	if id == "" || !ok {
		return chainEnd{}
	}
	seen := map[string]bool{id: true}
	for span.Parent != "" {
		if seen[span.Parent] {
			return chainEnd{}
		}
		seen[span.Parent] = true
		if at, ok := g.callOf[span.Parent]; ok && at != i {
			return chainEnd{parent: at, found: true}
		}
		if span, ok = g.spans[span.Parent]; !ok {
			return chainEnd{}
		}
	}
	return g.fromRoot(i, span)
}

func (g spanChains) fromRoot(i int, root Span) chainEnd {
	matched, count := 0, 0
	for _, at := range g.into[root.Service] {
		if at != i && servedRoot(g.calls[at], root) {
			matched, count = at, count+1
		}
	}
	if count == 1 {
		return chainEnd{parent: matched, found: true, route: true}
	}
	return chainEnd{rooted: true, root: root.Name}
}

func servedRoot(c Call, root Span) bool {
	if root.Method == "" || root.Route == "" || !strings.EqualFold(c.Method, root.Method) || !routeMatches(root.Route, c.Path) {
		return false
	}
	if c.Started.Add(-routeClockGap).After(root.Start) {
		return false
	}
	return c.Finished.IsZero() || !root.Start.After(c.Finished.Add(routeClockGap))
}

func routeMatches(route, path string) bool {
	want, got := routeParts(route), routeParts(path)
	if len(want) != len(got) {
		return false
	}
	for i, part := range want {
		variable := strings.HasPrefix(part, ":") || (strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}"))
		if !variable && part != got[i] {
			return false
		}
	}
	return true
}

func routeParts(path string) []string {
	return strings.Split(strings.Trim(path, "/"), "/")
}

func rootName(name string) string {
	runes := []rune(name)
	return string(runes[:min(len(runes), rootNameLimit)])
}

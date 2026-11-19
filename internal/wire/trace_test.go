package wire

import "testing"

const (
	clientSpan = "22f067aa0ba902b9"
	serverSpan = "33f067aa0ba902ba"
)

func span(id, parent, service, name string, ms int) Span {
	return NewSpan(id, parent, service, name, nil, at(ms))
}

func request(c Call, method, path string) Call {
	c.Method, c.Path = method, path
	return c
}

func TestASpanChainMakesTheRunnerCallTheParentAcrossUnrecordedSpans(t *testing.T) {
	calls := []Call{
		withSpan(call(1, "cockpit-runner", "orders", 10, 50), spanOne),
		withSpan(call(2, "orders", "ledger", 20, 30), clientSpan),
	}
	spans := []Span{span(serverSpan, spanOne, "orders", "POST /orders", 11), span(clientSpan, serverSpan, "orders", "http post", 20)}
	assertLinks(t, LinkCalls(calls, linkResult(), spans), []Link{
		{Kind: LinkExact, Step: 0},
		{Kind: LinkTraced, Step: 0, Parent: 1},
	})
}

func TestASpanChainOverrulesAnEnclosingCallThatOnlyLooksLikeTheParent(t *testing.T) {
	calls := []Call{
		request(call(1, "gateway", "orders", 205, 240), "POST", "/hooks/done"),
		withSpan(call(2, "cockpit-runner", "orders", 206, 250), spanTwo),
		withSpan(call(3, "orders", "ledger", 210, 215), clientSpan),
	}
	spans := []Span{span(serverSpan, "", "orders", "http post /hooks/done", 206), span(clientSpan, serverSpan, "orders", "http post", 210)}
	links := LinkCalls(calls, linkResult(), spans)
	if want := (Link{Kind: LinkTraced, Step: 1, Parent: 1, Route: true}); links[2] != want {
		t.Fatalf("got %+v want %+v", links[2], want)
	}
	if without := LinkCalls(calls, linkResult(), nil); without[2].Kind != LinkNested || without[2].Parent != 2 {
		t.Fatalf("canary: timing alone picks the enclosing runner call: %+v", without[2])
	}
}

func TestWorkATiedCallStartedStaysWithItsStepDuringALaterOne(t *testing.T) {
	calls := []Call{
		withSpan(call(1, "cockpit-runner", "orders", 10, 20), spanOne),
		withSpan(call(2, "orders", "ledger", 210, 220), clientSpan),
	}
	spans := []Span{span(clientSpan, spanOne, "orders", "worker", 210)}
	assertLinks(t, LinkCalls(calls, linkResult(), spans), []Link{{Kind: LinkExact, Step: 0}, {Kind: LinkTraced, Step: 0, Parent: 1}})
}

func TestAChildOfAnUntiedCallIsListedWhereItStarted(t *testing.T) {
	calls := []Call{
		request(call(1, "gateway", "orders", 90, 95), "POST", "/hooks/done"),
		withSpan(call(2, "orders", "ledger", 210, 220), clientSpan),
		withSpan(call(3, "orders", "ledger", 150, 151), spanTwo+"0"),
	}
	spans := []Span{
		span(serverSpan, "", "orders", "POST /hooks/done", 91), span(clientSpan, serverSpan, "orders", "client", 210),
		span(spanTwo+"0", serverSpan, "orders", "client", 150),
	}
	links := LinkCalls(calls, linkResult(), spans)
	if want := (Link{Kind: LinkTraced, Step: 1, Parent: 1, Route: true}); links[1] != want {
		t.Fatalf("inside a window: got %+v want %+v", links[1], want)
	}
	if want := (Link{Kind: LinkTraced, Step: 0, Parent: 1, Route: true}); links[2] != want {
		t.Fatalf("outside every window: got %+v want %+v", links[2], want)
	}
}

func TestARootReachesACallOnlyWhenExactlyOneOpenRequestMatchesIt(t *testing.T) {
	root := span(serverSpan, "", "orders", "POST /hooks/{id}", 20)
	child := withSpan(call(9, "orders", "ledger", 21, 22), clientSpan)
	inbound := func(method, path string, from, to int) Call {
		return request(call(1, "gateway", "orders", from, to), method, path)
	}
	cases := map[string]struct {
		calls []Call
		found bool
	}{
		"one match":            {[]Call{inbound("POST", "/hooks/7", 19, 30)}, true},
		"another method":       {[]Call{inbound("GET", "/hooks/7", 19, 30)}, false},
		"another route":        {[]Call{inbound("POST", "/other/7", 19, 30)}, false},
		"a longer path":        {[]Call{inbound("POST", "/hooks/7/x", 19, 30)}, false},
		"closed before":        {[]Call{inbound("POST", "/hooks/7", 5, 10)}, false},
		"started after":        {[]Call{inbound("POST", "/hooks/7", 25, 30)}, false},
		"two equal candidates": {[]Call{inbound("POST", "/hooks/7", 19, 30), inbound("POST", "/hooks/8", 18, 30)}, false},
	}
	for name, tc := range cases {
		calls := append(append([]Call{}, tc.calls...), child)
		for i := range tc.calls {
			calls[i].Seq = int64(i + 1)
		}
		got := LinkCalls(calls, linkResult(), []Span{root, span(clientSpan, serverSpan, "orders", "client", 21)})[len(calls)-1]
		if tc.found && (got.Kind != LinkTraced || got.Parent != 1 || !got.Route) {
			t.Errorf("%s: %+v", name, got)
		}
		if !tc.found && (got.Kind != LinkBackground || got.Root != root.Name) {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

func TestBackgroundWorkKeepsItsRootAndCanEncloseTheCallItMakes(t *testing.T) {
	calls := []Call{
		withSpan(call(1, "orders", "gateway", 10, 30), clientSpan),
		call(2, "gateway", "ledger", 12, 14),
	}
	spans := []Span{span(clientSpan, serverSpan, "orders", "client", 10), span(serverSpan, "", "orders", "task reconcile", 9)}
	assertLinks(t, LinkCalls(calls, linkResult(), spans), []Link{
		{Kind: LinkBackground, Step: 0, Root: "task reconcile"},
		{Kind: LinkNested, Step: 0, Parent: 1},
	})
}

func TestAChainThatBreaksOrLoopsIsNoEvidence(t *testing.T) {
	calls := []Call{withSpan(call(1, "orders", "ledger", 10, 20), clientSpan)}
	want := []Link{{Kind: LinkTime, Step: 0}}
	assertLinks(t, LinkCalls(calls, linkResult(), []Span{span(clientSpan, "ffffffffffffffff", "orders", "client", 10)}), want)
	loop := []Span{span(clientSpan, serverSpan, "orders", "a", 10), span(serverSpan, clientSpan, "orders", "b", 10)}
	assertLinks(t, LinkCalls(calls, linkResult(), loop), want)
	assertLinks(t, LinkCalls(calls, linkResult(), []Span{span(clientSpan, "", "orders", "root", 10)}), []Link{{Kind: LinkBackground, Step: 0, Root: "root"}})
}

func TestASpanReadsItsMethodAndRouteFromAttributesThenItsName(t *testing.T) {
	named := NewSpan("a", "", "s", "http post /hooks/{id}", nil, at(0))
	if named.Method != "POST" || named.Route != "/hooks/{id}" {
		t.Fatalf("from the name: %+v", named)
	}
	attrs := NewSpan("a", "", "s", "handler", map[string]any{"http.request.method": "PUT", "http.target": "/v1/x?y=1"}, at(0))
	if attrs.Method != "PUT" || attrs.Route != "/v1/x" {
		t.Fatalf("from the attributes: %+v", attrs)
	}
}

package screen

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/wire"
	"github.com/d0lim/cockpit/internal/wire/wiretest"
)

var wireT0 = time.Date(2026, 11, 19, 1, 0, 0, 0, time.UTC)

func wireHead(seq int64, caller, target, path string, from, reply, to float64, link wire.LinkKind) api.WireHead {
	at := func(ms float64) time.Time { return wireT0.Add(time.Duration(ms * float64(time.Millisecond))) }
	return api.WireHead{
		Seq: seq, Caller: caller, Target: target, Method: "GET", Path: path, Status: 200, Outcome: wire.OutcomeAnswered,
		Started: at(from), Replied: at(reply), Finished: at(to), DurationMs: int64(to - from), Link: link, Step: "s1",
	}
}

func child(h api.WireHead, parent int64) api.WireHead {
	h.Parent = parent
	return h
}

func noLinks() wireLinks {
	return wireLinks{call: func(seq int64) string { return "/call/" + itoa64(seq) }, unfold: func(seq int64) string { return "/open/" + itoa64(seq) }}
}

func eventOrder(m wireModel) string {
	var out []string
	for _, e := range seqEvents(m) {
		kind := "ask"
		if e.reply {
			kind = "answer"
		}
		out = append(out, kind+itoa64(e.node.head.Seq))
	}
	return strings.Join(out, " ")
}

func TestAnInnerCallIsAskedAndAnsweredBetweenItsParentsRequestAndAnswer(t *testing.T) {
	m := newWireModel([]api.WireHead{
		wireHead(1, wireRunner, "orders", "/a", 0, 20, 21, wire.LinkExact),
		child(wireHead(2, "orders", "ledger", "/b", 5, 10, 11, wire.LinkTraced), 1),
	}, false, false, nil)
	if got := eventOrder(m); got != "ask1 ask2 answer2 answer1" {
		t.Fatalf("events %s", got)
	}
	if got := strings.Join(m.lanes(), ","); got != "cockpit-runner,orders,ledger" {
		t.Fatalf("lanes %s", got)
	}
}

func TestARepeatedCallFoldsUnderItsParentAndOpensOnRequest(t *testing.T) {
	calls := []api.WireHead{wireHead(1, wireRunner, "orders", "/a", 0, 100, 101, wire.LinkExact)}
	for i := range 4 {
		from := float64(10 + i*10)
		calls = append(calls, child(wireHead(int64(i+2), "orders", "ledger", fmt.Sprintf("/poll/%d", i), from, from+1, from+2, wire.LinkTraced), 1))
	}
	folded := newWireModel(calls, false, false, nil)
	if len(folded.nodes) != 2 || len(folded.nodes[1].fold) != 4 || wireFoldEvery(folded.nodes[1].fold) != 10 {
		t.Fatalf("folded %+v", folded.nodes)
	}
	opened := newWireModel(calls, false, false, map[int64]bool{2: true})
	if len(opened.nodes) != 5 || opened.nodes[1].fold != nil {
		t.Fatalf("opened %d nodes", len(opened.nodes))
	}
	two := newWireModel(calls[:3], false, false, nil)
	if len(two.nodes) != 3 {
		t.Fatalf("canary: two repeats stay apart, got %d nodes", len(two.nodes))
	}
}

func TestAFailedCallIsNotFoldedWithTheCallsThatPassed(t *testing.T) {
	var calls []api.WireHead
	for i := range 3 {
		calls = append(calls, wireHead(int64(i+1), "orders", "ledger", "/a", float64(i), float64(i)+.5, float64(i)+.6, wire.LinkTime))
	}
	calls[1].Status = 503
	if m := newWireModel(calls, false, false, nil); len(m.nodes) != 3 {
		t.Fatalf("nodes %d", len(m.nodes))
	}
}

func TestTiedOnlyLeavesOutBackgroundAndTimeCallsAndTheirMark(t *testing.T) {
	calls := []api.WireHead{
		wireHead(1, wireRunner, "orders", "/a", 0, 5, 6, wire.LinkExact),
		wireHead(2, "orders", "ledger", "/b", 1, 2, 3, wire.LinkBackground),
		wireHead(3, "worker", "ledger", "/c", 1, 2, 3, wire.LinkTime),
	}
	if m := newWireModel(calls, true, false, nil); len(m.nodes) != 1 || m.nodes[0].head.Seq != 1 {
		t.Fatalf("tied only %d", len(m.nodes))
	}
	if m := newWireModel(calls, false, false, nil); len(m.nodes) != 3 {
		t.Fatalf("canary: all calls %d", len(m.nodes))
	}
}

func TestAParentOutsideTheSelectionIsNamedOnTheCall(t *testing.T) {
	m := newWireModel([]api.WireHead{child(wireHead(2, "orders", "ledger", "/b", 0, 1, 2, wire.LinkTraced), 77)}, false, false, nil)
	for name, scene := range map[string]wireScene{"sequence": wireSequence(m, noLinks()), "trace": wireTrace(m, noLinks())} {
		found := false
		for _, text := range scene.Texts {
			found = found || text.Tag == "↖#77 "
		}
		if !found {
			t.Errorf("%s does not name call 77", name)
		}
	}
}

func TestTheTraceSortsRootsByWhatStartedThem(t *testing.T) {
	bg := wireHead(4, "orders", "ledger", "/d", 3, 4, 5, wire.LinkBackground)
	bg.Root = "task reconcile"
	m := newWireModel([]api.WireHead{
		wireHead(1, wireRunner, "orders", "/a", 0, 5, 6, wire.LinkExact),
		wireHead(2, "gateway", "orders", "/b", 1, 2, 3, wire.LinkTime),
		child(wireHead(3, "orders", "ledger", "/c", 2, 3, 4, wire.LinkTraced), 2),
		bg, wireHead(5, "worker", "ledger", "/e", 4, 5, 6, wire.LinkTime),
		child(wireHead(6, "orders", "mail", "/f", 5, 6, 7, wire.LinkTraced), 99),
	}, false, false, nil)
	var groups []string
	joined := ""
	for _, text := range wireTrace(m, noLinks()).Texts {
		joined += text.Text + "\n"
		if text.Class == "wire-group wire-small" {
			groups = append(groups, text.Text)
		}
	}
	if got := strings.Join(groups, "|"); got != strings.Join(traceGroups, "|") {
		t.Fatalf("groups %s", got)
	}
	if !strings.Contains(joined, "task reconcile") {
		t.Fatalf("the background root is missing:\n%s", joined)
	}
}

func TestTheTraceAxisFoldsIdleTimeAndKeepsOrder(t *testing.T) {
	m := newWireModel([]api.WireHead{
		wireHead(1, wireRunner, "orders", "/a", 0, 10, 10, wire.LinkExact),
		wireHead(2, wireRunner, "ledger", "/b", 60000, 60010, 60010, wire.LinkExact),
	}, false, false, nil)
	axis := newTraceAxis(m)
	first, second := axis.x(10), axis.x(60000)
	if len(axis.segments) != 2 || second-first != traceBreak {
		t.Fatalf("segments %+v gap %v", axis.segments, second-first)
	}
	if end := axis.x(60010); end != axis.right {
		t.Fatalf("the last bar ends at %v, not %v", end, axis.right)
	}
}

func TestTheMermaidTextEscapesWhatWouldEndAStatement(t *testing.T) {
	first := wireHead(1, wireRunner, "orders", "/a;b#c%d\ne", 0, 5, 6, wire.LinkExact)
	late := wireHead(2, "worker", "orders", "/t", 7, 8, 9, wire.LinkTime)
	late.Status, late.Outcome = 0, wire.OutcomeNoUpstream
	want := `sequenceDiagram
    participant p0 as runner
    participant p1 as orders
    participant p2 as worker
    p0->>p1: GET /a#59;b#35;c#37;d e
    p1-->>p0: 200 · 6ms
    p2-)p1: GET /t
    p1--xp2: no upstream · 2ms
`
	if got := wireMermaid(newWireModel([]api.WireHead{first, late}, false, false, nil)); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestAFlowIsCutIntoStepsAndARepeatStaysInItsStep(t *testing.T) {
	var calls []api.WireHead
	for i := range 6 {
		h := wireHead(int64(i+1), wireRunner, "orders", "/a", float64(i), float64(i)+.5, float64(i)+.6, wire.LinkExact)
		h.Step = "one"
		if i >= 3 {
			h.Step = "two"
		}
		calls = append(calls, h)
	}
	m := newWireModel(calls, false, true, nil)
	if len(m.nodes) != 2 {
		t.Fatalf("each step keeps its own fold: %d nodes", len(m.nodes))
	}
	var bands []string
	scene := wireSequence(m, noLinks())
	for _, text := range scene.Texts {
		if text.Class == "wire-name" {
			bands = append(bands, text.Text)
		}
	}
	if strings.Join(bands, ",") != "step one,step two" || len(scene.Bands) != 2 {
		t.Fatalf("bands %v", bands)
	}
	if text := wireMermaid(m); !strings.Contains(text, "Note over p0,p1: step one\n") || !strings.Contains(text, "Note over p0,p1: step two\n") {
		t.Fatalf("mermaid\n%s", text)
	}
}

var wireInlineStyle = regexp.MustCompile(`<[^>]+\sstyle=`)

func TestTheScreenDrawsEveryCallAsALinkWithoutInlineStylesInBothViews(t *testing.T) {
	f, base := seededWire(t)
	for _, view := range []string{"", "&view=trace"} {
		_, _, body := f.get(base + "?flow=alpha&step=a2&call=5" + view)
		region := section(t, body, "wire")
		for _, seq := range []string{"4", "5", "6", "7"} {
			if !strings.Contains(region, `data-seq="`+seq+`"`) {
				t.Errorf("%q has no link for call %s", view, seq)
			}
		}
		if wireInlineStyle.MatchString(region) {
			t.Errorf("%q holds an inline style", view)
		}
		if !strings.Contains(region, "wire-hit-on") || !wireCallLink.MatchString(html.UnescapeString(region)) {
			t.Errorf("%q does not mark or link the picked call", view)
		}
	}
}

func TestTheFlowHeadingOpensTheWholeFlow(t *testing.T) {
	f, base := seededWire(t)
	_, _, plain := f.get(base)
	if !strings.Contains(html.UnescapeString(section(t, plain, "wire")), "flow=alpha&scope=flow") {
		t.Fatal("the flow heading links nowhere")
	}
	_, _, body := f.get(base + "?scope=flow&flow=alpha")
	region := section(t, body, "wire")
	if !strings.Contains(region, "step a1") || !strings.Contains(region, "step a2") || !strings.Contains(region, "Every step of this flow") {
		t.Fatal("the flow view lacks its steps")
	}
}

func TestADiagramOfAFullPageStaysWithinItsTimeBudget(t *testing.T) {
	f := newFixture(t)
	doneWire(f, 1)
	base := wiretest.Calls("x")[7]
	list := make([]wire.Call, api.WirePageSize)
	for i := range list {
		c := base
		c.Seq, c.Path = int64(i+1), "/late/"+strings.Repeat("x", i%40)+string(rune('a'+i%26))
		c.Started = wiretest.T0.Add(30*time.Second + time.Duration(i)*time.Millisecond)
		c.Finished = c.Started.Add(time.Millisecond)
		list[i] = c
	}
	wiretest.Store(t, f.runDir(rid(1)), wiretest.Take{Calls: list, Routes: wiretest.Routes()})
	for _, view := range []string{wireViewSequence, wireViewTrace} {
		start := time.Now()
		status, _, body := f.get("/runs/" + rid(1) + "/wire?view=" + view)
		if took := time.Since(start); status != 200 || took > 3*time.Second || !strings.Contains(body, "<svg") {
			t.Fatalf("%s: %d in %s", view, status, took)
		}
	}
}

func TestThePickedCallOpensInAPanelWhoseCloseLinkDropsOnlyTheCall(t *testing.T) {
	f, base := seededWire(t)
	for _, view := range []string{wireViewCalls, wireViewSequence, wireViewTrace} {
		_, _, body := f.get(base + "?flow=alpha&step=a2&call=5&view=" + view)
		panel := html.UnescapeString(section(t, body, "wire-call"))
		if !strings.Contains(panel, `class="wire-drawer"`) || !strings.Contains(panel, ">Call 5<") {
			t.Fatalf("%s: the call is not in the panel: %s", view, panel)
		}
		closeLink := regexp.MustCompile(`href="([^"]*)" hx-swap="innerHTML show:none" data-drawer-close`).FindStringSubmatch(panel)
		if closeLink == nil || strings.Contains(closeLink[1], "call=") || !strings.Contains(closeLink[1], "view="+view) || !strings.Contains(closeLink[1], "step=a2") {
			t.Fatalf("%s: close link %v", view, closeLink)
		}
		if n := strings.Count(section(t, body, "wire"), `data-scroll-here="call-5"`); n != 1 {
			t.Fatalf("%s: the picked call is marked to scroll into view %d times", view, n)
		}
	}
	_, _, none := f.get(base + "?flow=alpha&step=a2&view=" + wireViewCalls)
	if strings.Contains(none, `id="wire-call"`) || strings.Contains(none, "data-scroll-here") || !strings.Contains(none, `data-seq="5"`) {
		t.Fatal("without a picked call the panel or a scroll mark remains")
	}
}

package screen

import (
	"net/url"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

func deepTrace() []spanRow {
	at := func(second string) string { return "2026-10-19T01:00:" + second + ".000Z" }
	return spanRows([]contract.Signal{
		span("t", "pipeline", "", "pipeline", at("00"), at("20")),
		span("t", "suite", "pipeline", "suite", at("01"), at("19")),
		span("t", "run", "suite", "run", at("02"), at("18")),
		span("t", "alpha", "run", "alpha", at("03"), at("09")),
		span("t", "a1", "alpha", "a1", at("04"), at("05")),
		span("t", "a1-call", "a1", "GET /a", at("04"), at("05")),
		span("t", "a2", "alpha", "a2", at("06"), at("07")),
		span("t", "beta", "run", "beta", at("10"), at("17")),
		span("t", "b1", "beta", "b1", at("11"), at("12")),
	})
}

func foldShape(fold spanFold) string {
	var out []string
	for _, r := range fold.rows {
		mark := ""
		switch {
		case r.toggle != "" && r.open:
			mark = "-"
		case r.toggle != "":
			mark = "+" + itoa(r.below)
		}
		out = append(out, r.id+mark)
	}
	return strings.Join(out, " ")
}

func foldState(query string) *state {
	q, _ := url.ParseQuery(query)
	return &state{path: "/t", query: q}
}

func TestATraceOpensToItsFourthLevelAndOpenUnfoldsOneSpanAtATime(t *testing.T) {
	cases := map[string]string{
		"":                   "pipeline suite run alpha+3 beta+1",
		"open=alpha":         "pipeline suite run alpha- a1+1 a2 beta+1",
		"open=alpha&open=a1": "pipeline suite run alpha- a1- a1-call a2 beta+1",
		"open=a1":            "pipeline suite run alpha+3 beta+1",
		"span=a1-call":       "pipeline suite run alpha- a1- a1-call a2 beta+1",
		"open=all":           "pipeline suite run alpha a1 a1-call a2 beta b1",
	}
	for query, want := range cases {
		fold := foldState(query).foldSpans(deepTrace())
		if got := foldShape(fold); got != want || fold.total != 9 {
			t.Errorf("%q shows %q of %d, want %q", query, got, fold.total, want)
		}
	}
	if len(cases) != 6 {
		t.Fatal("case count")
	}
}

func TestASpanToggleAddsOrDropsItsIdAndClosingAnAncestorOfThePickDropsThePick(t *testing.T) {
	toggles := func(query string) map[string]string {
		out := map[string]string{}
		for _, r := range foldState(query).foldSpans(deepTrace()).rows {
			out[r.id] = r.toggle
		}
		return out
	}
	closed := toggles("trace=t")
	if closed["alpha"] != "/t?open=alpha&trace=t" || closed["pipeline"] != "" || closed["run"] != "" {
		t.Fatalf("closed rows: %v", closed)
	}
	open := toggles("trace=t&open=alpha&open=beta")
	if open["alpha"] != "/t?open=beta&trace=t" || open["a1"] != "/t?open=alpha&open=beta&open=a1&trace=t" || open["a2"] != "" {
		t.Fatalf("open rows: %v", open)
	}
	forced := toggles("trace=t&span=a1-call")
	if forced["alpha"] != "/t?trace=t" || forced["a1"] != "/t?trace=t" {
		t.Fatalf("the ancestors of the pick: %v", forced)
	}
}

func TestTheTraceViewListsTheTracesBesideTheSpansAndFoldsDeepOnes(t *testing.T) {
	f := newFixture(t)
	finished(f)
	at := func(second string) string { return "2026-10-19T01:00:" + second + ".000Z" }
	f.signals.add(
		span("t1", "pipeline", "", "pipeline", at("00"), at("20")),
		span("t1", "suite", "pipeline", "suite", at("01"), at("19")),
		span("t1", "run", "suite", "run", at("02"), at("18")),
		span("t1", "alpha", "run", "alpha", at("03"), at("09")),
		span("t1", "a1", "alpha", "deep-step", at("04"), at("05")),
		span("t2", "other", "", "other-root", at("30"), at("31")),
	)
	base := "/runs/" + rid(1) + "/signals/trace"
	_, _, folded := f.get(base + "?trace=t1")
	for _, want := range []string{`aria-label="Traces"`, ">pipeline<", ">other-root<", "4 of 5 spans shown", ">alpha<", "+1<", `open=alpha`, "Open every span"} {
		if !strings.Contains(folded, want) {
			t.Errorf("the folded trace lacks %q", want)
		}
	}
	if strings.Contains(folded, ">deep-step<") {
		t.Fatal("a span below the fourth level shows before its parent is opened")
	}
	_, _, opened := f.get(base + "?trace=t1&open=alpha")
	if !strings.Contains(opened, ">deep-step<") || !strings.Contains(opened, "5 of 5 spans shown") || strings.Contains(opened, "Open every span") {
		t.Fatal("open=alpha does not show the span under alpha")
	}
	_, _, picked := f.get(base + "?trace=t1&span=a1")
	if !strings.Contains(picked, ">deep-step<") || !strings.Contains(picked, `aria-label="Span detail"`) {
		t.Fatal("a picked span is not shown with its detail")
	}
	_, _, all := f.get(base + "?trace=t1&open=all")
	if !strings.Contains(all, ">deep-step<") || !strings.Contains(all, "Fold to four levels") {
		t.Fatal("open=all does not open every span")
	}
}

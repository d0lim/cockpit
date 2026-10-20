package screen

import (
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

func span(trace, id, parent, name, start, end string) contract.Signal {
	return contract.Signal{Kind: contract.SignalKindSpan, Collector: "runner", ID: id, TSSource: contract.TimeSourceRunner, Span: &contract.SpanBody{TraceID: trace, ParentID: parent, Name: name, Start: start, End: end, Status: contract.SpanStatusOK}}
}

func event(collector, status, label string, attrs contract.Attrs) contract.Signal {
	a := contract.Attrs{"status": status, "label": label}
	for k, v := range attrs {
		a[k] = v
	}
	return contract.Signal{Kind: contract.SignalKindEvent, Collector: collector, TS: "2026-10-19T01:00:05.000Z", Attrs: a, Event: &contract.EventBody{Body: label + " body"}}
}

func seeded(t *testing.T) *fixture {
	f := newFixture(t)
	finished(f)
	f.signals.add(
		span("t1", "root", "", "smoke", "2026-10-19T01:00:00.000Z", "2026-10-19T01:00:01.000Z"),
		span("t1", "f1", "root", "health", "2026-10-19T01:00:00.200Z", "2026-10-19T01:00:00.400Z"),
		event("services", "running", "app", contract.Attrs{"table": "services", "tone": "ok", "health": "healthy"}),
		event("services", "exited", "seed", contract.Attrs{"table": "services", "tone": "bad"}),
		event("logs", "", "", contract.Attrs{"container": "app"}),
	)
	return f
}

func TestEachSignalViewRendersWhatItsQuestionNeeds(t *testing.T) {
	f := seeded(t)
	base := "/runs/" + rid(1) + "/signals/"
	cases := map[string][]string{
		"trace":                      {">smoke<", ">health<", "1.0s", `class="span-track"`},
		"events":                     {"collector: services", "status: running", "app", "seed"},
		"events?f=status%3Aexited":   {"seed", "pill-on"},
		"table":                      {"services", "<th>health</th>", "healthy"},
		"table?hide=services.health": {"services", "<th>status</th>"},
		"logs":                       {"app · 1", " body"},
		"events?event=seq%3A3":       {"Event detail", "app body"},
		"trace?trace=t1&amp;span=f1": {},
	}
	for query, wants := range cases {
		_, _, body := f.get(base + strings.ReplaceAll(query, "&amp;", "&"))
		for _, w := range wants {
			if !strings.Contains(body, w) {
				t.Errorf("%s: missing %q", query, w)
			}
		}
	}
	_, _, filtered := f.get(base + "events?f=status%3Aexited")
	if strings.Contains(filtered, `>app<`) {
		t.Fatal("the status filter keeps a running event")
	}
	_, _, hidden := f.get(base + "table?hide=services.health")
	if strings.Contains(hidden, "<th>health</th>") {
		t.Fatal("a hidden column is still shown")
	}
	if len(cases) != 8 {
		t.Fatal("case count")
	}
}

func TestTheSignalCacheReadsOnlyNewSignalsAndKeepsTheLatestTwentyThousand(t *testing.T) {
	f := seeded(t)
	first := f.sc.signals.read(rid(1))
	if len(first.signals) != 5 {
		t.Fatalf("first read %d", len(first.signals))
	}
	f.signals.add(event("services", "running", "late", nil))
	second := f.sc.signals.read(rid(1))
	if len(second.signals) != 6 || second.signals[5].Attrs["label"] != "late" || len(first.signals) != 5 {
		t.Fatalf("second read %d", len(second.signals))
	}
	for range maxSignals {
		f.signals.add(event("pulse", "", "", nil))
	}
	third := f.sc.signals.read(rid(1))
	if len(third.signals) != maxSignals || third.signals[0].Collector != "pulse" {
		t.Fatalf("third read %d, first %s", len(third.signals), third.signals[0].Collector)
	}
}

func TestTheSignalCacheForgetsRunsNobodyReadForTenMinutes(t *testing.T) {
	f := seeded(t)
	now := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	f.sc.signals.now = func() time.Time { return now }
	f.sc.signals.read(rid(1))
	f.sc.signals.read(rid(2))
	now = now.Add(11 * time.Minute)
	f.sc.signals.read(rid(2))
	if _, kept := f.sc.signals.runs[rid(1)]; kept {
		t.Fatal("an idle run stays in the cache")
	}
	if _, kept := f.sc.signals.runs[rid(2)]; !kept {
		t.Fatal("canary: the run just read is gone")
	}
}

func TestLatestRevisionsKeepOneVersionPerIdButEveryMetricAndUnnamedSignal(t *testing.T) {
	a1 := contract.Signal{Seq: 1, Kind: contract.SignalKindEvent, Collector: "c", ID: "a", Rev: 1}
	a2 := contract.Signal{Seq: 2, Kind: contract.SignalKindEvent, Collector: "c", ID: "a", Rev: 2}
	m1 := contract.Signal{Seq: 3, Kind: contract.SignalKindMetric, Collector: "c", ID: "m"}
	m2 := contract.Signal{Seq: 4, Kind: contract.SignalKindMetric, Collector: "c", ID: "m"}
	plain := contract.Signal{Seq: 5, Kind: contract.SignalKindEvent, Collector: "c"}
	got := latestRevisions([]contract.Signal{a1, a2, m1, m2, plain})
	var seqs []int64
	for _, s := range got {
		seqs = append(seqs, s.Seq)
	}
	if len(seqs) != 4 || seqs[0] != 2 || seqs[3] != 5 {
		t.Fatalf("%v", seqs)
	}
}

func TestSpanRowsNestChildrenUnderTheirParentAndScaleToTheTrace(t *testing.T) {
	spans := []contract.Signal{
		span("t", "c", "r", "child", "2026-10-19T01:00:00.500Z", "2026-10-19T01:00:01.000Z"),
		span("t", "r", "", "root", "2026-10-19T01:00:00.000Z", "2026-10-19T01:00:01.000Z"),
		span("t", "o", "", "open", "2026-10-19T01:00:00.250Z", ""),
	}
	rows := spanRows(spans)
	if len(rows) != 3 || rows[0].id != "r" || rows[1].id != "c" || rows[1].depth != 1 || rows[2].id != "o" || rows[2].depth != 0 {
		t.Fatalf("%+v", rows)
	}
	if rows[1].offset != 50 || rows[1].width != 50 || !rows[2].marker || rows[2].width != 0.5 {
		t.Fatalf("child %+v open %+v", rows[1], rows[2])
	}
	list := traceList(spans)
	if len(list) != 1 || list[0].root != "root" || *list[0].duration != 1000 || list[0].spans != 3 {
		t.Fatalf("%+v", list)
	}
}

func TestEmptyKindExplainsWhyAViewHasNothing(t *testing.T) {
	up := api.RunItem{Phase: lifecycle.Done, Env: lifecycle.EnvUp}
	armed := []api.CollectorState{{ID: "c", State: "armed"}}
	stopped := []api.CollectorState{{ID: "c", State: "stopped"}}
	cases := []struct {
		run        api.RunItem
		collectors []api.CollectorState
		count      int
		total      int
		want       string
	}{
		{api.RunItem{Phase: lifecycle.Building}, armed, 0, 0, "before-boot"},
		{api.RunItem{Phase: lifecycle.Done, Env: lifecycle.EnvStopped}, armed, 3, 3, "env-off"},
		{up, nil, 0, 0, "no-collectors"},
		{up, armed, 0, 4, "none-of-kind"},
		{up, stopped, 0, 0, "collectors-stopped"},
		{up, armed, 0, 0, "no-signals"},
		{up, armed, 2, 2, ""},
	}
	for i, c := range cases {
		if got := emptyKind(c.run, c.collectors, c.count, c.total); got != c.want {
			t.Errorf("case %d: %q, want %q", i, got, c.want)
		}
		if c.want != "" && emptySentences[c.want] == "" {
			t.Errorf("case %d: no sentence for %q", i, c.want)
		}
	}
	if len(cases) != 7 {
		t.Fatal("case count")
	}
}

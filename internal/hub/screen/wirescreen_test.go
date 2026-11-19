package screen

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/wire"
	"github.com/d0lim/cockpit/internal/wire/wiretest"
)

var wireRowSeq = regexp.MustCompile(`data-seq="(\d+)"`)

func rowSeqs(html string) []string {
	var out []string
	for _, m := range wireRowSeq.FindAllStringSubmatch(html, -1) {
		out = append(out, m[1])
	}
	return out
}

func section(t *testing.T, html, id string) string {
	t.Helper()
	start := strings.Index(html, `<section id="`+id+`"`)
	if start < 0 {
		t.Fatalf("no section %s", id)
	}
	end := strings.Index(html[start:], "</section>")
	return html[start : start+end]
}

func TestTheWireTabAndPathExistOnlyForARunThatRecorded(t *testing.T) {
	f := newFixture(t)
	finished(f)
	doneWire(f, 2)
	wiretest.Seed(t, f.runDir(rid(2)), planted)
	_, _, plain := f.get("/runs/" + rid(1))
	if strings.Contains(plain, `/runs/`+rid(1)+`/wire"`) || strings.Contains(plain, "wire recorder") {
		t.Fatal("a run without wire shows the tab or the notice")
	}
	if status, _, _ := f.get("/runs/" + rid(1) + "/wire"); status != http.StatusNotFound {
		t.Fatalf("a run without wire answers %d", status)
	}
	status, _, body := f.get("/runs/" + rid(2) + "/wire")
	if status != http.StatusOK || !strings.Contains(body, `aria-current="page">Wire<`) || !strings.Contains(body, `id="wire"`) {
		t.Fatalf("canary: the recorded run: %d", status)
	}
}

func TestOverviewReportAndTheFlowStepSayARunRecorded(t *testing.T) {
	f, base := seededWire(t)
	run := "/runs/" + rid(1)
	writeBundle(t, f.runDir(rid(1)))
	writeRunner(t, f.runDir(rid(1)), "boom")
	for _, path := range []string{run, run + "/report"} {
		_, _, body := f.get(path)
		if !strings.Contains(body, "passed through the wire recorder") || !strings.Contains(body, "The record is stored.") {
			t.Errorf("%s has no wire notice", path)
		}
	}
	os.RemoveAll(filepath.Join(f.runDir(rid(1)), report.DirReport))
	_, _, flow := f.get(run + "/report/alpha?step=a1")
	if !strings.Contains(flow, `href="`+base+`?flow=alpha&amp;step=a1"`) {
		t.Fatal("the picked step has no link to the Wire screen")
	}
}

func TestThePlainScreenOpensOnTheFailedStepWithItsStepListAndRouteCounts(t *testing.T) {
	f, base := seededWire(t)
	_, _, body := f.get(base + "?view=calls")
	region := section(t, body, "wire")
	if got := strings.Join(rowSeqs(region), ","); got != "4,5,6,7" {
		t.Fatalf("rows %s", got)
	}
	for _, want := range []string{"alpha › a2", "Outside any step", `3 calls in this take">orders<`, `4 calls in this take">ledger<`, `1 call in this take">mail<`, "Calls 1 to 4 of 4", `aria-current="true"`} {
		if !strings.Contains(region, want) {
			t.Errorf("the region lacks %q", want)
		}
	}
	if strings.Contains(region, `id="wire-call"`) {
		t.Fatal("the picked call must sit outside the region")
	}
	if strings.Contains(body, "No call went through") {
		t.Fatal("canary: every route has calls here")
	}
}

func TestNestedRowsIndentUnderTheirParentAndTimeRowsStandApart(t *testing.T) {
	f, base := seededWire(t)
	_, _, body := f.get(base + "?view=calls&flow=alpha&step=a2")
	region := section(t, body, "wire")
	heading := strings.Index(region, "not tied to it")
	if heading < 0 {
		t.Fatal("no time block")
	}
	before, after := rowSeqs(region[:heading]), rowSeqs(region[heading:])
	if strings.Join(before, ",") != "4,5,6" || strings.Join(after, ",") != "7" {
		t.Fatalf("linked %v, by time %v", before, after)
	}
	for _, want := range []string{`data-seq="5" data-link="nested" data-depth="1"`, `data-seq="6" data-link="nested" data-depth="2"`, "pl-4", "pl-8", `data-seq="7" data-link="time"`} {
		if !strings.Contains(region, want) {
			t.Errorf("the region lacks %q", want)
		}
	}
	if !strings.Contains(region[heading:], ">time</span>") || strings.Contains(region[:heading], ">time</span>") {
		t.Fatal("only rows matched by time carry the badge")
	}
}

func TestEachBodyKindAndAWithheldCallReadInWords(t *testing.T) {
	f, base := seededWire(t)
	cases := []struct {
		call string
		want []string
	}{
		{"1", []string{"cockpit-runner", "orders:8080", "The runner sent this request for step alpha › a1", "&#34;id&#34;: &#34;ord-1&#34;"}},
		{"2", []string{"limit = 5", "orders was answering call 1", ">ok</pre>"}},
		{"5", []string{"amount", ">7</div>", "Decoded JWT, signature not checked and not stored", ">Claims</p>", "&#34;sub&#34;: &#34;user-1&#34;"}},
		{"6", []string{"Body not kept: 2.1 MiB, over the 1 MiB limit"}},
		{"7", []string{"failed the check that nothing secret remains", "(not kept)"}},
	}
	for _, c := range cases {
		status, _, body := f.get(base + "?call=" + c.call)
		panel := section(t, body, "wire-call")
		if status != http.StatusOK || strings.Contains(panel, planted) {
			t.Fatalf("call %s: %d", c.call, status)
		}
		for _, want := range c.want {
			if !strings.Contains(panel, want) {
				t.Errorf("call %s lacks %q in %s", c.call, want, panel)
			}
		}
	}
	if len(cases) != 5 {
		t.Fatal("case count")
	}
	_, _, withheld := f.get(base + "?call=7")
	if strings.Contains(section(t, withheld, "wire-call"), "Request") {
		t.Fatal("a withheld call shows no request")
	}
}

func TestAReloadWithTheQueryShowsTheSameCallAndThePickedRow(t *testing.T) {
	f, base := seededWire(t)
	path := base + "?view=calls&flow=alpha&step=a2&target=ledger&call=5"
	_, _, first := f.get(path)
	_, _, again := f.get(path)
	if section(t, first, "wire-call") != section(t, again, "wire-call") || section(t, first, "wire") != section(t, again, "wire") {
		t.Fatal("a reload shows something else")
	}
	if !strings.Contains(first, `<tr class="row-on" data-seq="5"`) || !strings.Contains(first, ">Call 5<") {
		t.Fatal("canary: the picked call is not marked")
	}
	if !strings.Contains(section(t, first, "wire"), "target=ledger") {
		t.Fatal("the row links lost the route filter")
	}
}

func TestTheLiveRegionRendersFromTheIndexAndNeverReadsABody(t *testing.T) {
	f, base := seededWire(t)
	screen, err := f.sc.Resolve(context.Background(), base+"?flow=alpha&step=a2&call=4")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.runDir(rid(1)), wire.StagingDir, wire.SuiteTake, wire.CallsFile)); err != nil {
		t.Fatal(err)
	}
	raw, err := screen.Render(context.Background(), []string{"wire"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	if !strings.Contains(out, `data-seq="4"`) || strings.Contains(out, "cannot be read") || strings.Contains(out, `id="wire-call"`) {
		t.Fatalf("region: %s", out)
	}
	if status, _, page := f.get(base + "?flow=alpha&step=a2&call=4"); status != http.StatusOK || !strings.Contains(page, "This call cannot be shown") {
		t.Fatalf("canary: the picked call needs its line: %d", status)
	}
}

func TestStatesWithoutARecordSayWhatHappened(t *testing.T) {
	f := newFixture(t)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}, doneUp, wireRun))
	_, _, pending := f.get("/runs/" + rid(1) + "/wire")
	if !strings.Contains(pending, "The record is fixed when the run reaches its report.") || strings.Contains(pending, "Outside any step") {
		t.Fatal("pending")
	}
	doneWire(f, 2)
	writeBundle(t, f.runDir(rid(2)))
	path := filepath.Join(f.runDir(rid(2)), report.DirReport, report.FileSummary)
	raw, _ := os.ReadFile(path)
	var s contract.Summary
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	s.Wire = &contract.WireStatus{Take: wire.SuiteTake, State: wire.StateLost, Detail: "the recorder was gone"}
	raw, _ = json.Marshal(s)
	os.WriteFile(path, raw, 0o600)
	_, _, lost := f.get("/runs/" + rid(2) + "/wire")
	if !strings.Contains(lost, "The record of the suite was not stored (lost): the recorder was gone.") {
		t.Fatal("lost")
	}
	f.seed(runMeta(3, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictFailed}, doneUp, wireRun, func(m *store.Meta) {
		m.Redaction = &store.Redaction{Status: report.RedactionFailed, Files: []string{"meta.json"}}
	}))
	wiretest.Store(t, f.runDir(rid(3)), wiretest.Take{Calls: wiretest.Calls("x")[:1], Routes: wiretest.Routes(), Result: wiretest.Result()})
	_, _, discarded := f.get("/runs/" + rid(3) + "/wire")
	for _, want := range []string{"discarded the report bundle", "No call went through ledger (COCKPIT_WIRE_LEDGER), mail (COCKPIT_WIRE_MAIL)"} {
		if !strings.Contains(discarded, want) {
			t.Errorf("the notice lacks %q", want)
		}
	}
}

func TestTheTakePickerAppearsOnlyWithASecondTakeAndAnUnknownStepSaysSo(t *testing.T) {
	f, base := seededWire(t)
	_, _, one := f.get(base)
	if strings.Contains(one, `aria-label="Takes"`) {
		t.Fatal("one take needs no picker")
	}
	wiretest.Store(t, f.runDir(rid(1)), wiretest.Take{Name: "exec-1", Calls: wiretest.Calls("x")[:2], Routes: wiretest.Routes()})
	_, _, two := f.get(base + "?view=calls&take=exec-1&scope=outside")
	if !strings.Contains(two, `aria-label="Takes"`) || strings.Join(rowSeqs(section(t, two, "wire")), ",") != "1,2" {
		t.Fatalf("two takes: %v", rowSeqs(section(t, two, "wire")))
	}
	_, _, missing := f.get(base + "?flow=alpha&step=ghost")
	if !strings.Contains(missing, "this take has no such step") {
		t.Fatal("an unknown step")
	}
}

func TestAPageOfManyCallsStaysWithinItsRowAndTimeBudget(t *testing.T) {
	const calls, rowBudget = 2500, 200
	timeBudget := 3 * time.Second
	f := newFixture(t)
	doneWire(f, 1)
	base := wiretest.Calls("x")[7]
	list := make([]wire.Call, calls)
	for i := range list {
		c := base
		c.Seq = int64(i + 1)
		c.Path = "/late/" + strconv.Itoa(i)
		c.Started = wiretest.T0.Add(30*time.Second + time.Duration(i)*time.Millisecond)
		c.Finished = c.Started.Add(time.Millisecond)
		list[i] = c
	}
	wiretest.Store(t, f.runDir(rid(1)), wiretest.Take{Calls: list, Routes: wiretest.Routes()})
	path := "/runs/" + rid(1) + "/wire?view=calls"
	start := time.Now()
	status, _, body := f.get(path)
	took := time.Since(start)
	rows := rowSeqs(section(t, body, "wire"))
	if status != http.StatusOK || len(rows) != rowBudget || !strings.Contains(body, "Calls 1 to 200 of 2500") || !strings.Contains(body, "Later calls") {
		t.Fatalf("%d, %d rows", status, len(rows))
	}
	if took > timeBudget {
		t.Fatalf("the page took %s, over %s", took, timeBudget)
	}
	t.Logf("first page of %d calls: %d rows, %d bytes, %s", calls, len(rows), len(body), took)
	_, _, next := f.get(path + "&offset=200")
	if got := rowSeqs(section(t, next, "wire")); len(got) != rowBudget || got[0] != "201" || !strings.Contains(next, "Earlier calls") {
		t.Fatalf("page two starts at %v", got[:1])
	}
}

var wireCallLink = regexp.MustCompile(`href="[^"#]*[?&;]call=\d+[^"#]*"`)

func TestACallLinkPicksTheCallWithoutAnAnchorAndTheSuiteTakeIsListedFirst(t *testing.T) {
	f, base := seededWire(t)
	wiretest.Store(t, f.runDir(rid(1)), wiretest.Take{Name: "2026-exec", Calls: wiretest.Calls("x")[:1], Routes: wiretest.Routes()})
	_, _, body := f.get(base + "?flow=alpha&step=a2")
	region := section(t, body, "wire")
	if !wireCallLink.MatchString(region) {
		t.Fatalf("no call link picks a call: %s", region)
	}
	suite, exec := strings.Index(region, ">suite<"), strings.Index(region, ">2026-exec<")
	if suite < 0 || exec < 0 || suite > exec {
		t.Fatalf("the takes read suite at %d and the exec at %d", suite, exec)
	}
	if strings.Contains(region, "ledger ·") || !strings.Contains(region, `in this take">ledger<`) {
		t.Fatalf("the route pill counts the whole take beside the calls of one step: %s", region)
	}
}

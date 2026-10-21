package screen

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/signal"
)

func row(table, id, body string) contract.Signal {
	return contract.Signal{Kind: contract.SignalKindEvent, Collector: "toy-db", ID: table + ":" + id, TS: "2026-10-19T01:00:06.000Z", Attrs: contract.Attrs{"table": table, "label": table}, Event: &contract.EventBody{Body: body}}
}

func markedStep() contract.Signal {
	s := span(rid(1), "step:alpha/a1", "flow:alpha", "a1", "2026-10-19T01:00:02.000Z", "2026-10-19T01:00:03.000Z")
	s.Attrs = contract.Attrs{"flow": "alpha", "step": "a1", "status": "passed", "ref.order": "ord-7"}
	return s
}

func correlated(t *testing.T) *fixture {
	f := newFixture(t)
	finished(f)
	f.signals.add(
		markedStep(),
		row("orders", "ord-7", `{"id":"ord-7","state":"open","amount":"5"}`),
		row("orders", "ord-8", `{"id":"ord-8","state":"shut","amount":"9"}`),
		event("notify", "delivered", "order opened", contract.Attrs{"key": "ord-7"}),
	)
	return f
}

func TestATableRowLinksBackToTheStepThatMarkedAnIdInItsBody(t *testing.T) {
	f := correlated(t)
	_, _, body := f.get("/runs/" + rid(1) + "/signals/table")
	for _, want := range []string{"<th>step</th>", "<th>id</th>", "<th>state</th>", "<th>amount</th>", ">open<", ">shut<", `href="/runs/` + rid(1) + `/report/alpha?step=a1"`} {
		if !strings.Contains(body, want) {
			t.Errorf("table view lacks %q", want)
		}
	}
	if strings.Index(body, "<th>id</th>") > strings.Index(body, "<th>state</th>") {
		t.Error("body columns lost their order")
	}
	if n := strings.Count(body, `/report/alpha?step=a1"`); n != 1 {
		t.Fatalf("%d step links, want only the ord-7 row", n)
	}
}

func TestAPickedEventListsTheStepAndTheOtherSignalsThatCarryItsIds(t *testing.T) {
	f := correlated(t)
	key := url.QueryEscape("toy-db/orders:ord-7")
	_, _, body := f.get("/runs/" + rid(1) + "/signals/events?event=" + key)
	for _, want := range []string{"Related by id", "Marked by", ">alpha/a1<", "order opened", "q=ord-7", "&#34;state&#34;: &#34;open&#34;"} {
		if !strings.Contains(body, want) {
			t.Errorf("event detail lacks %q", want)
		}
	}
	_, _, other := f.get("/runs/" + rid(1) + "/signals/events?event=" + url.QueryEscape("toy-db/orders:ord-8"))
	if strings.Contains(other, "Related by id") || !strings.Contains(other, "Event detail") {
		t.Fatal("an unmarked row lists related signals, or the canary detail is missing")
	}
}

func TestTheEventSearchKeepsOnlyEventsContainingTheTextAcrossAllEvents(t *testing.T) {
	f := correlated(t)
	_, _, hit := f.get("/runs/" + rid(1) + "/signals/events?q=ORD-8")
	if !strings.Contains(hit, "orders") || strings.Contains(hit, "order opened") {
		t.Fatalf("search for ord-8 shows the wrong events")
	}
	_, _, miss := f.get("/runs/" + rid(1) + "/signals/events?q=nothing-here")
	if !strings.Contains(miss, "No event contains") {
		t.Fatal("an empty search says nothing")
	}
}

func TestTheFlowScreenShowsThePickedStepsRequestsIdsAndWhatCarriesThem(t *testing.T) {
	f := newFixture(t)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}, doneUp))
	dir := f.runDir(rid(1))
	writeRunner(t, dir, "status 500")
	created := 201
	var result contract.Result
	raw, _ := os.ReadFile(filepath.Join(dir, api.RunnerDir, "result.json"))
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	result.Steps[0].Calls = []contract.StepCall{{Method: "POST", Host: "app", Path: "/orders", Status: &created, StartedAt: "2026-10-19T01:00:02.100Z", DurationMs: 40}}
	result.Steps[0].Refs = []contract.StepRef{{Name: "order", Value: "ord-7"}}
	writeJSON(t, filepath.Join(dir, api.RunnerDir, "result.json"), result)
	f.signals.add(markedStep(), row("orders", "ord-7", `{"id":"ord-7","state":"open","amount":"5"}`))
	_, _, picked := f.get("/runs/" + rid(1) + "/report/alpha?step=a1")
	for _, want := range []string{"Requests", "POST /orders", ">201<", "40ms", "Ids this step marked", "ord-7", "Related by id", "orders · orders:ord-7", "span=call%3Aalpha%2Fa1%2F1"} {
		if !strings.Contains(picked, want) {
			t.Errorf("flow screen lacks %q", want)
		}
	}
	if strings.Contains(picked, ">alpha/a1<") {
		t.Error("the step lists itself as marking its own id")
	}
	_, _, unpicked := f.get("/runs/" + rid(1) + "/report/alpha")
	if strings.Contains(unpicked, "POST /orders") || !strings.Contains(unpicked, ">a1<") {
		t.Fatal("the records show without a picked step, or the canary step row is missing")
	}
}

func TestADestroyedRunsSignalsComeFromItsBundleAndSaySo(t *testing.T) {
	f := newFixture(t)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvDestroyed, Verdict: lifecycle.VerdictPassed}, doneUp))
	dir := filepath.Join(f.runDir(rid(1)), report.DirReport, signal.Dir)
	os.MkdirAll(dir, 0o700)
	line := `{"seq":1,"ts":"2026-10-19T01:00:05.000Z","tsSource":"host","kind":"event","collector":"toy-db","id":"orders:1","attrs":{"table":"orders"},"event":{"body":"{\"id\":\"ord-1\"}"}}` + "\n"
	os.WriteFile(filepath.Join(dir, "stream.jsonl"), []byte(line), 0o600)
	f.signals.destroyed = true
	_, _, body := f.get("/runs/" + rid(1) + "/signals/table")
	if !strings.Contains(body, "frozen into its report bundle") || !strings.Contains(body, ">ord-1<") {
		t.Fatalf("destroyed run signals: %s", body)
	}
}

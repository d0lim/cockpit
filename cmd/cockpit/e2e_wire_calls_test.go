package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/wire"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

type wireStepCalls struct {
	flow, step string
	calls      []string
}

const (
	byRunnerToApp = "cockpit-runner app "
	byAppToLedger = "app ledger "
)

func runnerCall(method, path string) string { return byRunnerToApp + method + " " + path + " exact" }

var (
	wireUpCalls = []wireStepCalls{
		{"up", "app-health", []string{runnerCall("GET", "/health")}},
		{"up", "ledger-health", []string{"cockpit-runner ledger GET /health exact"}},
	}
	wireRelayStepCalls = []wireStepCalls{
		{"relay", "child-span", []string{runnerCall("POST", "/relay"), byAppToLedger + "POST /entries nested /relay"}},
		{"relay", "no-span", []string{runnerCall("POST", "/relay"), byAppToLedger + "POST /entries nested /relay"}},
		{"relay", "same-span", []string{runnerCall("POST", "/relay"), byAppToLedger + "POST /entries exact"}},
	}
	wireRelayCalls  = slices.Concat(wireUpCalls, wireRelayStepCalls)
	wireRecallCalls = slices.Concat(wireUpCalls, []wireStepCalls{
		{"recall", "latest", []string{runnerCall("GET", "/tickets/latest")}},
	})
	wireSuiteCalls = slices.Concat(wireRelayCalls, []wireStepCalls{
		{"timing", "schedule", []string{runnerCall("POST", "/later")}},
		{"timing", "hook-window", []string{byAppToLedger + "POST /hooks/late time"}},
		{"callback", "register", []string{runnerCall("POST", "/register"), byAppToLedger + "POST /hooks/done nested /register"}},
		{"credentials", "env-token", []string{runnerCall("POST", "/echo")}},
		{"credentials", "private-key", []string{runnerCall("POST", "/echo")}},
		{"credentials", "path-secret", []string{runnerCall("GET", "/accounts/****")}},
		{"credentials", "service-secret", []string{runnerCall("POST", "/secret-relay"), byAppToLedger + "POST /echo nested /secret-relay"}},
		{"credentials", "shapes", []string{runnerCall("GET", "/shapes")}},
		{"learned", "issue", []string{runnerCall("POST", "/tickets")}},
		{"learned", "redeem", []string{runnerCall("POST", "/tickets/redeem")}},
		{"recall", "latest", []string{runnerCall("GET", "/tickets/latest")}},
		{"bodies", "jwt", []string{runnerCall("GET", "/jwt")}},
		{"bodies", "pem-begin", []string{runnerCall("GET", "/pem/begin")}},
		{"bodies", "pem-end", []string{runnerCall("GET", "/pem/end")}},
		{"bodies", "big", []string{runnerCall("GET", "/big")}},
		{"bodies", "binary", []string{runnerCall("GET", "/binary")}},
		{"closing", "schedule-after", []string{runnerCall("POST", "/later")}},
	})
	wireOutsideCalls = []string{byAppToLedger + "GET /boot none", byAppToLedger + "POST /hooks/after none"}
)

func describeHeads(heads []api.WireHead) []string {
	bySeq := map[int64]api.WireHead{}
	for _, h := range heads {
		bySeq[h.Seq] = h
	}
	out := make([]string, 0, len(heads))
	for _, h := range heads {
		text := fmt.Sprintf("%s %s %s %s %s", h.Caller, h.Target, h.Method, h.Path, h.Link)
		if h.Link == wire.LinkNested {
			text += " " + bySeq[h.Parent].Path
		}
		out = append(out, text)
	}
	return out
}

func (w wireRun) assertTake(take string, table []wireStepCalls, outside int) {
	t := w.e.t
	t.Helper()
	view := w.view("?take=" + take)
	if view.Record == nil || view.Record.State != wire.StateTaken {
		t.Fatalf("take %s record %+v", take, view.Record)
	}
	listed := make([]string, 0, len(view.Steps))
	for _, s := range view.Steps {
		listed = append(listed, s.Flow+"/"+s.Step)
	}
	want := make([]string, 0, len(table))
	total := outside
	routes := map[string]int{}
	for _, row := range table {
		want = append(want, row.flow+"/"+row.step)
		total += len(row.calls)
		got := w.view("?take=" + take + "&flow=" + row.flow + "&step=" + row.step)
		lines := describeHeads(got.Calls)
		if !slices.Equal(lines, row.calls) {
			t.Errorf("take %s step %s/%s calls\n got %q\nwant %q", take, row.flow, row.step, lines, row.calls)
		}
		for _, h := range got.Calls {
			routes[h.Target]++
			if h.Outcome != wire.OutcomeAnswered || h.Status == 0 || h.Withheld {
				t.Errorf("take %s call %d reads %+v", take, h.Seq, h)
			}
		}
	}
	slices.Sort(listed)
	slices.Sort(want)
	if !slices.Equal(listed, want) {
		t.Errorf("take %s lists the steps %q, the table has %q", take, listed, want)
	}
	if view.Record.Calls != total || view.Record.WithheldCalls != 0 || view.Record.CallsDropped != 0 {
		t.Errorf("take %s record %+v, want %d calls and none withheld or dropped", take, view.Record, total)
	}
	if view.Outside != outside {
		t.Errorf("take %s has %d calls outside the steps, want %d", take, view.Outside, outside)
	}
	if outside > 0 {
		lines := describeHeads(w.view("?take=" + take + "&scope=outside").Calls)
		if !slices.Equal(lines, wireOutsideCalls) {
			t.Errorf("take %s outside the steps\n got %q\nwant %q", take, lines, wireOutsideCalls)
		}
		routes["ledger"] += outside
	}
	for _, r := range view.Routes {
		if r.Calls != routes[r.Name] {
			t.Errorf("take %s route %s counts %d calls, the table %d", take, r.Name, r.Calls, routes[r.Name])
		}
	}
	if len(view.Routes) != 2 {
		t.Errorf("take %s has %d routes", take, len(view.Routes))
	}
}

func (w wireRun) assertSuiteTake() {
	t := w.e.t
	t.Helper()
	w.assertTake(wire.SuiteTake, wireSuiteCalls, len(wireOutsideCalls))
	s := w.summary().Wire
	if s == nil || s.State != wire.StateTaken || s.Calls != 29 || s.BodiesOmitted != 2 {
		t.Errorf("summary wire %+v", s)
	}
	assertBodies(t, w)
}

func (w wireRun) headOf(take, flow, step string, at int) api.WireHead {
	w.e.t.Helper()
	calls := w.view("?take=" + take + "&flow=" + flow + "&step=" + step).Calls
	if at >= len(calls) {
		w.e.t.Fatalf("step %s/%s has %d calls", flow, step, len(calls))
	}
	return calls[at]
}

func (w wireRun) picked(flow, step string, at int) wire.Call {
	w.e.t.Helper()
	return w.pickedIn(wire.SuiteTake, flow, step, at)
}

func (w wireRun) pickedIn(take, flow, step string, at int) wire.Call {
	w.e.t.Helper()
	return w.call(take, w.headOf(take, flow, step, at).Seq).Call
}

func (w wireRun) publicTicketID() string {
	w.e.t.Helper()
	body := w.picked("learned", "issue", 0).Response.Body
	id, _ := body.JSON.(map[string]any)["id"].(string)
	if !strings.HasPrefix(id, "ord-") {
		w.e.t.Fatalf("the ticket id reads %q in %+v", id, body)
	}
	return id
}

package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
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

func TestAnUnknownTakeSeqStepRouteAndBadQueriesAreRefused(t *testing.T) {
	f, base := seededWire(t)
	cases := map[string]int{
		base + "?take=ghost":               http.StatusNotFound,
		base + "?take=../../etc":           http.StatusNotFound,
		base + "/99":                       http.StatusNotFound,
		base + "/1?take=ghost":             http.StatusNotFound,
		base + "/nope":                     http.StatusBadRequest,
		base + "?flow=alpha&step=ghost":    http.StatusNotFound,
		base + "?flow=alpha":               http.StatusBadRequest,
		base + "?scope=everything":         http.StatusBadRequest,
		base + "?scope=outside&flow=alpha": http.StatusBadRequest,
		base + "?target=ghost":             http.StatusNotFound,
		base + "?offset=-1":                http.StatusBadRequest,
		base + "?take=suite":               http.StatusOK,
		base + "/1?take=suite":             http.StatusOK,
	}
	for path, want := range cases {
		if status, body := f.get(path); status != want {
			t.Errorf("%s: %d, want %d: %s", path, status, want, body)
		}
	}
	if len(cases) != 13 {
		t.Fatal("case count")
	}
}

func TestTheViewCountsStepsAndEveryRouteIncludingOneWithoutCalls(t *testing.T) {
	f, base := seededWire(t)
	v := getWire(t, f, base)
	if v.Status != WireReady || v.State != wire.StateTaken || v.Take != wire.SuiteTake || v.Record.Calls != 8 || v.Record.WithheldCalls != 1 || v.Outside != 1 {
		t.Fatalf("view %+v", v)
	}
	want := map[string]int{"orders": 3, "ledger": 4, "mail": 1}
	if len(v.Routes) != 3 {
		t.Fatalf("routes %+v", v.Routes)
	}
	for _, r := range v.Routes {
		if r.Calls != want[r.Name] {
			t.Errorf("route %s: %d calls, want %d", r.Name, r.Calls, want[r.Name])
		}
	}
	steps := map[string]WireStep{}
	for _, s := range v.Steps {
		steps[s.Flow+"/"+s.Step] = s
	}
	a1, a2, b1 := steps["alpha/a1"], steps["alpha/a2"], steps["beta/b1"]
	if a1.Exact != 1 || a1.Nested != 1 || a1.Time != 1 || a2.Exact != 1 || a2.Nested != 2 || a2.Time != 1 || b1.Calls() != 0 || a2.Status != contract.StepStatusFailed {
		t.Fatalf("steps %+v", v.Steps)
	}
	empty := getWire(t, f, base+"?flow=beta&step=b1&target=mail")
	if empty.Total != 0 || len(empty.Calls) != 0 {
		t.Fatalf("a step without calls: %+v", empty)
	}
	mail := getWire(t, f, base+"?flow=alpha&step=a2&target=mail")
	if mail.Total != 1 || mail.Calls[0].Seq != 6 || mail.Calls[0].Depth != 2 || mail.Calls[0].Parent != 5 {
		t.Fatalf("filtered by route: %+v", mail)
	}
}

func TestThePlainViewPicksTheFailedStepThenTheFirstWithCallsThenOutside(t *testing.T) {
	f, base := seededWire(t)
	v := getWire(t, f, base)
	if v.Scope != WireScopeStep || v.Flow != "alpha" || v.Step != "a2" || !reflect.DeepEqual(seqs(v.Calls), []int64{4, 5, 6, 7}) || v.Total != 4 {
		t.Fatalf("the failed step: %+v", v)
	}
	out := getWire(t, f, base+"?scope=outside")
	if out.Scope != WireScopeOutside || out.Flow != "" || !reflect.DeepEqual(seqs(out.Calls), []int64{8}) {
		t.Fatalf("outside: %+v", out)
	}

	f.seed(runMeta(2, wireDone, doneUp, wireMeta))
	passed := wiretest.Result()
	passed.Steps[1].Status = contract.StepStatusPassed
	wiretest.Store(t, f.runDir(rid(2)), wiretest.Take{Calls: wiretest.Calls("x"), Result: passed})
	if v := getWire(t, f, "/api/runs/"+rid(2)+"/wire"); v.Step != "a1" {
		t.Fatalf("no failed step: %+v", v)
	}

	f.seed(runMeta(3, wireDone, doneUp, wireMeta))
	wiretest.Store(t, f.runDir(rid(3)), wiretest.Take{Calls: wiretest.Calls("x")})
	if v := getWire(t, f, "/api/runs/"+rid(3)+"/wire"); v.Scope != WireScopeOutside || v.Total != 8 {
		t.Fatalf("no steps at all: %+v", v)
	}
}

func TestPagesStayWithinTheirBoundsAndKeepStartOrder(t *testing.T) {
	f := wireFixture(t)
	f.seed(runMeta(1, wireDone, doneUp, wireMeta))
	calls := make([]wire.Call, 0, 600)
	for i := range 600 {
		c := wiretest.Calls("x")[7]
		c.Seq = int64(600 - i)
		c.Started = c.Started.Add(-time.Duration(i) * time.Millisecond)
		c.Finished = c.Started
		calls = append(calls, c)
	}
	wiretest.Store(t, f.runDir(rid(1)), wiretest.Take{Calls: calls})
	base := "/api/runs/" + rid(1) + "/wire?scope=outside"
	cases := []struct {
		query        string
		count, first int
		offset, size int
	}{
		{"", 200, 1, 0, 200},
		{"&limit=10&offset=5", 10, 6, 5, 10},
		{"&limit=9999", 500, 1, 0, 500},
		{"&limit=500&offset=550", 50, 551, 550, 500},
		{"&offset=600", 0, 0, 600, 200},
		{"&offset=99999", 0, 0, 99999, 200},
	}
	for _, c := range cases {
		v := getWire(t, f, base+c.query)
		first := 0
		if len(v.Calls) > 0 {
			first = int(v.Calls[0].Seq)
		}
		if len(v.Calls) != c.count || first != c.first || v.Total != 600 || v.Offset != c.offset || v.Limit != c.size {
			t.Errorf("%q: %d calls, first %d, total %d, offset %d, limit %d", c.query, len(v.Calls), first, v.Total, v.Offset, v.Limit)
		}
	}
	if len(cases) != 6 {
		t.Fatal("case count")
	}
}

func TestADestroyedRunAndADiscardedBundleStillOpenTheirRecord(t *testing.T) {
	f := wireFixture(t)
	gone := lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvDestroyed, Verdict: lifecycle.VerdictFailed}
	f.seed(runMeta(1, gone, doneUp, wireMeta))
	dir := f.runDir(rid(1))
	writeBundle(t, dir)
	wiretest.Seed(t, dir, planted)
	record := wire.OpenStore(dir)
	if err := record.Attach(); err != nil {
		t.Fatal(err)
	}
	if record.Root() != filepath.Join(dir, report.DirReport, wire.StagingDir) {
		t.Fatalf("canary: the record is at %s", record.Root())
	}
	v := getWire(t, f, "/api/runs/"+rid(1)+"/wire")
	if v.Status != WireReady || v.Total != 4 || v.Discarded {
		t.Fatalf("destroyed: %+v", v)
	}
	if call, text := getWireCall(t, f, "/api/runs/"+rid(1)+"/wire/4"); call.Head.Seq != 4 || strings.Contains(text, planted) || !strings.Contains(text, "****") {
		t.Fatalf("destroyed call: %s", text)
	}

	f.seed(runMeta(2, wireDone, doneUp, wireMeta, func(m *store.Meta) {
		m.Redaction = &store.Redaction{Status: report.RedactionFailed, Files: []string{"meta.json"}}
	}))
	wiretest.Seed(t, f.runDir(rid(2)), planted)
	if v := getWire(t, f, "/api/runs/"+rid(2)+"/wire"); v.Status != WireReady || !v.Discarded || v.Total != 4 {
		t.Fatalf("discarded: %+v", v)
	}
}

func TestARunWithoutATakeSaysWhyFromItsLifecycleAndItsSummary(t *testing.T) {
	f := wireFixture(t)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}, doneUp, wireMeta))
	if v := getWire(t, f, "/api/runs/"+rid(1)+"/wire"); v.Status != WirePending || len(v.Takes) != 0 || v.State != "" || len(v.Calls) != 0 {
		t.Fatalf("active: %+v", v)
	}
	f.seed(runMeta(2, wireDone, doneUp, wireMeta))
	dir := f.runDir(rid(2))
	writeBundle(t, dir)
	path := filepath.Join(dir, report.DirReport, report.FileSummary)
	raw, _ := os.ReadFile(path)
	var s contract.Summary
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	s.Wire = &contract.WireStatus{Take: wire.SuiteTake, State: wire.StateLost, Detail: "gone " + planted}
	raw, _ = json.Marshal(s)
	os.WriteFile(path, raw, 0o600)
	v := getWire(t, f, "/api/runs/"+rid(2)+"/wire")
	if v.Status != WireNone || v.State != wire.StateLost || v.Detail != "gone "+maskedPlanted {
		t.Fatalf("lost: %+v", v)
	}
}

func TestAnExecTakeIsReadByNameAndEachLinkExplainsItself(t *testing.T) {
	f, base := seededWire(t)
	wiretest.Store(t, f.runDir(rid(1)), wiretest.Take{Name: "exec-1", Calls: wiretest.Calls("x")[:1], Routes: wiretest.Routes()})
	v := getWire(t, f, base+"?take=exec-1")
	if !reflect.DeepEqual(v.Takes, []string{"exec-1", "suite"}) || v.Take != "exec-1" || v.Total != 1 || v.Scope != WireScopeOutside {
		t.Fatalf("exec take: %+v", v)
	}
	want := map[int64]string{
		1: "The runner sent this request for step alpha › a1, and it carried span " + wiretest.SpanFirst + ".",
		2: "orders was answering call 1 (cockpit-runner to orders) for the whole of this call, so it belongs to step alpha › a1.",
		3: "This call started while step alpha › a1 ran, and nothing else ties it to the step.",
		8: "This call started outside every step.",
	}
	for seq, why := range want {
		call, _ := getWireCall(t, f, base+"/"+strconv.FormatInt(seq, 10))
		if call.Why != why {
			t.Errorf("call %d: %q", seq, call.Why)
		}
	}
	hidden, _ := getWireCall(t, f, base+"/7")
	if !hidden.Call.Withheld || hidden.Call.Path != "" || hidden.Head.Path != "" || hidden.Flow != "alpha" || hidden.Step != "a2" {
		t.Fatalf("withheld: %+v", hidden)
	}
}
func TestTheListIsServedFromTheParsedIndexWithoutTheBodiesAndFollowsANewVersion(t *testing.T) {
	f, base := seededWire(t)
	dir := filepath.Join(f.runDir(rid(1)), wire.StagingDir, wire.SuiteTake)
	if v := getWire(t, f, base); v.Total != 4 {
		t.Fatalf("first read: %+v", v)
	}
	if err := os.Remove(filepath.Join(dir, wire.CallsFile)); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(dir, wire.IndexFile)
	info, err := os.Stat(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(index, []byte(strings.Repeat(" ", int(info.Size()))), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(index, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if v := getWire(t, f, base); v.Total != 4 {
		t.Fatalf("the list did not come from the parsed index: %+v", v)
	}
	if status, _ := f.get(base + "/4"); status != http.StatusNotFound {
		t.Fatalf("canary: a call needs its line, got %d", status)
	}
	later := info.ModTime().Add(time.Minute)
	if err := os.Chtimes(index, later, later); err != nil {
		t.Fatal(err)
	}
	if status, body := f.get(base); status != http.StatusInternalServerError {
		t.Fatalf("a new version must be parsed again: %d %s", status, body)
	}
}

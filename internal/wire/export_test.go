package wire

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var exportMutations = []porttest.Mutation{
	{"a leaving call passes the proof with the masker of the read", "write the stored call in Export", "red: the planted value is in the calls", "TestExportProvesEachCallAgainAndRebuildsTheIndexFromTheCallsThatLeave"},
	{"a leaving head is built from the proven call", "keep the stored head in Export", "red: the planted value is in a head's path", "TestExportProvesEachCallAgainAndRebuildsTheIndexFromTheCallsThatLeave"},
	{"the step, route and root names pass the masker", "copy the stored steps in Export", "red: the planted value is in a step name", "TestExportProvesEachCallAgainAndRebuildsTheIndexFromTheCallsThatLeave"},
	{"the public fields of the take apply to the read", "pass the masker without the public fields to Prove", "red: the public token reads masked", "TestExportProvesEachCallAgainAndRebuildsTheIndexFromTheCallsThatLeave"},
	{"a call that no longer proves clean leaves as a stub", "use Clean in place of Prove in Export", "red: the PEM pair across the path and the body leaves", "TestExportWithholdsACallThatNoLongerProvesCleanAndCountsIt"},
	{"an index that does not match its calls is refused", "skip the seq comparison in storedCall", "red: another call leaves under the head", "TestExportRefusesAnIndexThatDoesNotMatchItsCalls"},
}

func TestExportMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "export_test.go", exportMutations)
}

func readMasker(t *testing.T, known ...string) *mask.Masker {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: known})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func storedRecord(t *testing.T, index Index, calls ...Call) (Index, *bytes.Reader) {
	t.Helper()
	var file bytes.Buffer
	for _, c := range calls {
		line := lineOf(t, c)
		head := headOf(c, int64(file.Len()), int64(len(line)))
		head.Link = Link{Kind: LinkExact, Step: 0, Root: "task " + planted}
		index.Heads = append(index.Heads, head)
		file.WriteString(line + "\n")
	}
	index.Status = Status{Take: SuiteTake, State: StateTaken, Calls: len(calls)}
	return index, bytes.NewReader(file.Bytes())
}

func leaving(t *testing.T, out *bytes.Buffer, head Head) Call {
	t.Helper()
	text := out.Bytes()
	var line Line
	if head.Offset+head.Length > int64(len(text)) || json.Unmarshal(text[head.Offset:head.Offset+head.Length], &line) != nil || line.Call == nil || line.Call.Seq != head.Seq {
		t.Fatalf("head %d does not point at its call in %s", head.Seq, text)
	}
	return *line.Call
}

func TestExportProvesEachCallAgainAndRebuildsTheIndexFromTheCallsThatLeave(t *testing.T) {
	first := Call{Seq: 1, Caller: "orders", Target: "ledger", Port: 8081, Method: "POST", Path: "/v1/entries/" + planted, Started: callStart, Finished: callStart.Add(4e6), Status: 200, Outcome: OutcomeAnswered,
		Request: Message{Size: 40, Body: Body{Kind: BodyJSON, JSON: map[string]any{"memo": planted, "token": publicToken}}}}
	second := Call{Seq: 2, Caller: "ledger", Target: "mail", Port: 2525, Method: "POST", Path: "/send", Started: callStart, Finished: callStart.Add(2e6), Status: 202, Outcome: OutcomeAnswered,
		Response: Message{Size: 9, Body: Body{Kind: BodyText, Text: "queued"}}}
	stored, calls := storedRecord(t, Index{
		Steps:        []StepRef{{Flow: "alpha", Step: "a-" + planted, Status: contract.StepStatusFailed}},
		Routes:       []contract.WireRoute{{Name: "ledger", Service: "ledger-" + planted, Port: 8081}},
		PublicFields: []contract.WirePublicField{{Key: "token", Value: "tok-[0-9a-f]{8,64}"}},
	}, first, second)
	plain, err := json.Marshal(stored)
	if err != nil || !strings.Contains(string(plain), planted) {
		t.Fatalf("the fixture plants nothing: %v", err)
	}
	var out bytes.Buffer
	index, err := Export(readMasker(t, planted), stored, calls, &out)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	assertMasked(t, "calls", out.String(), planted)
	assertMasked(t, "index", string(encoded), planted)
	for label, text := range map[string]string{"head path": index.Heads[0].Path, "step": index.Steps[0].Step, "route": index.Routes[0].Service, "root": index.Heads[0].Link.Root} {
		assertMasked(t, label, text, planted)
	}
	got := leaving(t, &out, index.Heads[0])
	if got.Request.Body.JSON.(map[string]any)["token"] != publicToken || leaving(t, &out, index.Heads[1]).Response.Body.Text != "queued" {
		t.Fatalf("the public field or an untouched body changed: %s", out.String())
	}
	if len(index.Heads) != 2 || index.Heads[1].Offset != index.Heads[0].Length+1 || index.Heads[0].Length == stored.Heads[0].Length {
		t.Fatalf("heads %+v", index.Heads)
	}
	if index.Status.Calls != 2 || index.Status.WithheldCalls != 0 || index.Heads[0].Link.Kind != LinkExact || index.Heads[0].DurationMs != 4 {
		t.Fatalf("status %+v link %+v", index.Status, index.Heads[0].Link)
	}
}

func TestExportWithholdsACallThatNoLongerProvesCleanAndCountsIt(t *testing.T) {
	split := Call{Seq: 1, Caller: "orders", Target: "ledger", Port: 8081, Method: "POST", Path: "/keys/" + pemBegin, Started: callStart, Status: 200, Outcome: OutcomeAnswered,
		Request: Message{Size: 30, Body: Body{Kind: BodyText, Text: "tail " + pemEnd}}}
	whole := Call{Seq: 2, Caller: "orders", Target: "ledger", Port: 8081, Method: "GET", Path: "/v1/entries", Started: callStart, Status: 200, Outcome: OutcomeAnswered}
	stored, calls := storedRecord(t, Index{Steps: []StepRef{{Flow: "alpha", Step: "a1"}}}, split, whole)
	var out bytes.Buffer
	index, err := Export(readMasker(t), stored, calls, &out)
	if err != nil {
		t.Fatal(err)
	}
	stub := leaving(t, &out, index.Heads[0])
	if !stub.Withheld || stub.Path != "" || stub.Request.Body.Kind != "" || strings.Contains(out.String(), "BEGIN KEY") || strings.Contains(out.String(), "END KEY") {
		t.Fatalf("the call left as %s", out.String())
	}
	if !index.Heads[0].Withheld || index.Heads[0].Path != "" || index.Heads[0].Link.Root != "" || index.Status.WithheldCalls != 1 {
		t.Fatalf("head %+v status %+v", index.Heads[0], index.Status)
	}
	if kept := leaving(t, &out, index.Heads[1]); kept.Withheld || kept.Path != "/v1/entries" || index.Heads[1].Link.Root == "" {
		t.Fatalf("the clean call left as %+v", kept)
	}
}

func TestExportRefusesAnIndexThatDoesNotMatchItsCalls(t *testing.T) {
	a := Call{Seq: 1, Caller: "orders", Target: "ledger", Method: "GET", Path: "/a", Started: callStart, Outcome: OutcomeAnswered}
	b := Call{Seq: 2, Caller: "orders", Target: "ledger", Method: "GET", Path: "/b", Started: callStart, Outcome: OutcomeAnswered}
	stored, calls := storedRecord(t, Index{}, a, b)
	var out bytes.Buffer
	if _, err := Export(readMasker(t), stored, calls, &out); err != nil || out.Len() == 0 {
		t.Fatalf("a matching index: %v", err)
	}
	crossed := stored
	crossed.Heads = []Head{stored.Heads[0], stored.Heads[1]}
	crossed.Heads[0].Offset, crossed.Heads[0].Length = stored.Heads[1].Offset, stored.Heads[1].Length
	for label, index := range map[string]Index{"crossed": crossed, "past the end": {Heads: []Head{{Seq: 1, Offset: 4096, Length: 10}}}, "no length": {Heads: []Head{{Seq: 1}}}} {
		if _, err := Export(readMasker(t), index, calls, &bytes.Buffer{}); err == nil {
			t.Errorf("%s: the export went through", label)
		}
	}
}

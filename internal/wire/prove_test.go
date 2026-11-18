package wire

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const (
	pemBegin = "-----BEGIN KEY-----"
	pemEnd   = "-----END KEY-----"
)

var proveMutations = []porttest.Mutation{
	{"Clean is a fixed point over stored lines", "append to the path in Clean", "red: a second Clean changes the line", "TestCleanChangesNothingInItsOwnOutputAcrossTheCorpusAndAfterStorage"},
	{"Clean keeps only what the body kind owns", "return the body unchanged from cleanBody", "red: a stray JSON value under a text kind stays", "TestCleanDropsWhatABodyKindDoesNotOwnAndReplacesUnknownKinds"},
	{"the line must pass the value scan", "drop the value scan from proven", "red: a PEM pair across one call is stored", "TestACallWhoseLineFailsTheValueScanBecomesAStubWithoutPathHeadersOrBodies"},
	{"proof uses the value scan and not the full masker", "scan with the record masker in proven", "red: a public field fails the proof", "TestAPublicFieldPassesTheProofThatTheFullMaskerWouldRefuse"},
	{"each call is proven alone", "keep the BEGIN of an earlier call in the scan", "red: a pair across two calls withholds them", "TestABeginInOneCallAndAnEndInAnotherPassTheirOwnProofs"},
	{"the record masker reaches values the capture did not know", "skip Clean in Prove", "red: a value captured at run time stays", "TestProveMasksAValueTheCaptureMaskerDidNotKnow"},
	{"a stub keeps only its identifying fields", "keep the path in stub", "red: the stub carries the path", "TestACallWhoseLineFailsTheValueScanBecomesAStubWithoutPathHeadersOrBodies"},
	{"a stub whose labels fail the scan loses them", "skip the label replacement in Prove", "red: a PEM pair in the labels is stored", "TestAStubWhoseLabelsFormAPEMPairLosesThem"},
	{"the scan skips the numbers the recorder wrote", "scan the whole line in proven", "red: a known number inside a nanosecond timestamp withholds the call", "TestAKnownNumberInsideTheRecordersOwnFieldsLeavesTheCallProven"},
	{"the scan still reads what a caller sent", "leave the path out of the scanned line too", "red: a PEM pair across the path and a body is stored", "TestAPEMPairAcrossThePathAndABodyIsWithheldThoughTheRecorderNumbersAreSkipped"},
	{"a call that was not cleaned fails the fixed-point check", "drop the second Clean from proven", "red: the planted value is stored", "TestProvingACallThatWasNotCleanedWithholdsIt"},
	{"a call that cannot be encoded is withheld", "treat an encoding failure as proven", "red: the unencodable call passes", "TestACallThatCannotBeEncodedIsWithheld"},
}

func TestProveMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "prove_test.go", proveMutations)
}

func TestCleanChangesNothingInItsOwnOutputAcrossTheCorpusAndAfterStorage(t *testing.T) {
	maskers := map[string]*mask.Masker{"record": recordMasker(t), "plain": newMasker(t)}
	hub, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted, plantedDigit}})
	if err != nil {
		t.Fatal(err)
	}
	maskers["hub"] = hub
	corpus := surfaces()
	executed := 0
	for name, m := range maskers {
		for _, s := range corpus {
			x := baseExchange()
			s.edit(&x)
			first := Capture(m, x)
			line := lineOf(t, first)
			if again := lineOf(t, Clean(m, first)); again != line {
				t.Errorf("%s/%s: Clean changed\n%s\n%s", name, s.name, line, again)
			}
			var stored Line
			if err := json.Unmarshal([]byte(line), &stored); err != nil {
				t.Fatalf("%s/%s: %v", name, s.name, err)
			}
			if after := lineOf(t, Clean(m, *stored.Call)); after != line {
				t.Errorf("%s/%s: Clean after storage changed\n%s\n%s", name, s.name, line, after)
			}
			executed++
		}
	}
	if executed != 63 {
		t.Fatalf("executed %d cases", executed)
	}
}

func TestCleanDropsWhatABodyKindDoesNotOwnAndReplacesUnknownKinds(t *testing.T) {
	m := recordMasker(t)
	cases := []struct {
		name string
		in   Body
		want Body
	}{
		{"text with a stray JSON value", Body{Kind: BodyText, Text: "ok", JSON: "PLANTED-stray"}, Body{Kind: BodyText, Text: "ok"}},
		{"JSON with stray text", Body{Kind: BodyJSON, JSON: "ok", Text: "PLANTED-stray"}, Body{Kind: BodyJSON, JSON: "ok"}},
		{"omitted with stray text", Body{Kind: BodyOmitted, Omitted: OmitBusy, Text: "PLANTED-stray"}, omitted(OmitBusy)},
		{"no kind with stray text", Body{Text: "PLANTED-stray"}, Body{}},
		{"an unknown kind", Body{Kind: "raw", Text: "PLANTED-stray"}, omitted(OmitBinary)},
		{"an unknown reason", Body{Kind: BodyOmitted, Omitted: "PLANTED-stray"}, omitted(OmitBinary)},
		{"a JWT without its parts", Body{Kind: BodyJWT}, omitted(OmitBinary)},
	}
	for _, c := range cases {
		if got := Clean(m, Call{Request: Message{Body: c.in}}).Request.Body; !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
	if got := Clean(m, Call{Outcome: "PLANTED-stray"}).Outcome; got != OutcomeIncomplete {
		t.Errorf("an unknown outcome stayed: %q", got)
	}
	if len(cases) != 7 {
		t.Fatalf("evaluated %d cases", len(cases))
	}
}

func splitPEMExchange() Exchange {
	x := baseExchange()
	x.Path = "/v1/visible-path"
	x.RawQuery = "page=2"
	x.Request = typed("text/plain", pemBegin+"\nkeep-out-request")
	x.Request.Headers["X-Request-Id"] = []string{"req-visible"}
	x.Response = typed("text/plain", "keep-out-response\n"+pemEnd)
	return x
}

func TestACallWhoseLineFailsTheValueScanBecomesAStubWithoutPathHeadersOrBodies(t *testing.T) {
	m := recordMasker(t)
	x := splitPEMExchange()
	got, ok := Prove(m, Capture(m, x))
	if ok {
		t.Fatal("a PEM pair across one call passed the proof")
	}
	want := Call{Seq: 1, Caller: "orders", Target: "ledger", Port: 8081, Method: "POST", Started: x.Started, Finished: x.Finished, Status: 200, Outcome: OutcomeAnswered, Withheld: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stub %+v, want %+v", got, want)
	}
	line := lineOf(t, got)
	for _, gone := range []string{"visible-path", "page", "req-visible", "keep-out", "BEGIN", "END", "headers"} {
		if strings.Contains(line, gone) {
			t.Errorf("%q reached the stub %s", gone, line)
		}
	}
	x.Response = typed("text/plain", "keep-out-response")
	proven, ok := Prove(m, Capture(m, x))
	if !ok || proven.Withheld || proven.Path != "/v1/visible-path" {
		t.Fatalf("the same call without the END: %v %+v", ok, proven)
	}
}

func TestAPublicFieldPassesTheProofThatTheFullMaskerWouldRefuse(t *testing.T) {
	m := recordMasker(t)
	x := baseExchange()
	x.Response = jsonRequest(`{"token":"` + publicToken + `"}`)
	got, ok := Prove(m, Capture(m, x))
	if !ok {
		t.Fatalf("the public field failed the proof: %+v", got)
	}
	line := lineOf(t, got)
	if !strings.Contains(line, `"token":"`+publicToken+`"`) {
		t.Fatalf("the public field is gone: %s", line)
	}
	if m.Mask(line) == line {
		t.Fatal("the full masker leaves the line alone, so the test proves nothing")
	}
}

func TestABeginInOneCallAndAnEndInAnotherPassTheirOwnProofs(t *testing.T) {
	m := recordMasker(t)
	first, second := baseExchange(), baseExchange()
	first.Response = typed("text/plain", pemBegin+"\nmiddle")
	second.Seq = 2
	second.Response = typed("text/plain", "middle\n"+pemEnd)
	a, okA := Prove(m, Capture(m, first))
	b, okB := Prove(m, Capture(m, second))
	if !okA || !okB || a.Withheld || b.Withheld {
		t.Fatalf("proofs %v %v", okA, okB)
	}
	joined := lineOf(t, a) + "\n" + lineOf(t, b)
	if m.ValueScan().Mask(joined) == joined {
		t.Fatal("the two lines together do not match the PEM rule, so the test proves nothing")
	}
}

func TestProveMasksAValueTheCaptureMaskerDidNotKnow(t *testing.T) {
	x := baseExchange()
	x.Response = jsonRequest(`{"code":"` + planted + `"}`)
	captured := Capture(newMasker(t), x)
	if !strings.Contains(lineOf(t, captured), planted) {
		t.Fatal("the capture masker already hid the value, so the test proves nothing")
	}
	got, ok := Prove(recordMasker(t), captured)
	if !ok {
		t.Fatalf("not proven: %+v", got)
	}
	assertMasked(t, "proven line", lineOf(t, got), planted)
}

func TestAStubWhoseLabelsFormAPEMPairLosesThem(t *testing.T) {
	m := recordMasker(t)
	c := Call{Seq: 4, Caller: pemBegin, Target: pemEnd, Method: "GET", Started: callStart, Status: 200, Outcome: OutcomeAnswered}
	got, ok := Prove(m, c)
	if ok || !got.Withheld {
		t.Fatalf("proof %v, call %+v", ok, got)
	}
	line := lineOf(t, got)
	assertMasked(t, "stub labels", line, "BEGIN")
	if got.Seq != 4 || got.Status != 200 {
		t.Fatalf("stub %+v", got)
	}
	plain := Call{Seq: 5, Caller: "orders", Target: "ledger", Method: "GET", Started: callStart, Finished: callStart.Add(time.Millisecond), Withheld: true}
	if kept, _ := Prove(m, plain); kept.Caller != "orders" || kept.Target != "ledger" || kept.Method != "GET" {
		t.Fatalf("a stub with clean labels lost them: %+v", kept)
	}
}

func TestACallThatCannotBeEncodedIsWithheld(t *testing.T) {
	m := recordMasker(t)
	c := Call{Seq: 6, Caller: "orders", Target: "ledger", Method: "GET", Started: callStart, Request: Message{Body: Body{Kind: BodyJSON, JSON: json.Number("not-a-number")}}}
	got, ok := Prove(m, c)
	if ok || !got.Withheld || got.Request.Body.Kind != "" {
		t.Fatalf("proof %v, call %+v", ok, got)
	}
	if got.Caller != "orders" {
		t.Fatalf("the stub lost its caller: %+v", got)
	}
}

func TestAKnownNumberInsideTheRecordersOwnFieldsLeavesTheCallProven(t *testing.T) {
	const known = "773668"
	m := newMasker(t, known)
	at := time.Date(2026, 11, 18, 9, 0, 0, 773668511, time.UTC)
	base := Call{Seq: 7, Caller: "orders", Target: "ledger", Port: 8081, Method: "GET", Path: "/v1/entries", Started: callStart, Replied: callStart.Add(time.Millisecond), Finished: callStart.Add(2 * time.Millisecond), Status: 200, Outcome: OutcomeAnswered}
	cases := map[string]func(*Call){
		"seq":           func(c *Call) { c.Seq = 773668 },
		"started":       func(c *Call) { c.Started = at },
		"replied":       func(c *Call) { c.Replied = at },
		"finished":      func(c *Call) { c.Finished = at },
		"request size":  func(c *Call) { c.Request.Size = 773668 },
		"response size": func(c *Call) { c.Response.Size = 773668 },
	}
	executed := 0
	for name, edit := range cases {
		c := base
		edit(&c)
		line := lineOf(t, c)
		if !strings.Contains(line, known) {
			t.Fatalf("%s: the line does not hold the number, so the case proves nothing: %s", name, line)
		}
		got, ok := Prove(m, c)
		if !ok || got.Withheld || got.Path != "/v1/entries" {
			t.Errorf("%s: proof %v, call %+v", name, ok, got)
		}
		if stored := lineOf(t, got); stored != line {
			t.Errorf("%s: the stored line changed\n%s\n%s", name, line, stored)
		}
		executed++
	}
	if executed != len(cases) || executed != 6 {
		t.Fatalf("executed %d cases", executed)
	}
	inBody := base
	inBody.Response.Body = Body{Kind: BodyText, Text: "code " + known}
	got, ok := Prove(m, inBody)
	if !ok {
		t.Fatalf("not proven: %+v", got)
	}
	assertMasked(t, "the number in a body", lineOf(t, got), known)
}

func TestAPEMPairAcrossThePathAndABodyIsWithheldThoughTheRecorderNumbersAreSkipped(t *testing.T) {
	m := recordMasker(t)
	c := Call{Seq: 8, Caller: "orders", Target: "ledger", Method: "GET", Path: "/v1/" + pemBegin, Started: callStart, Status: 200, Outcome: OutcomeAnswered, Response: Message{Body: Body{Kind: BodyText, Text: pemEnd}}}
	got, ok := Prove(m, c)
	if ok || !got.Withheld || got.Path != "" {
		t.Fatalf("proof %v, call %+v", ok, got)
	}
	c.Response.Body.Text = "no end here"
	kept, ok := Prove(m, c)
	if !ok || kept.Withheld || !strings.Contains(kept.Path, "BEGIN") {
		t.Fatalf("the same call without the END: %v %+v", ok, kept)
	}
}

func TestProvingACallThatWasNotCleanedWithholdsIt(t *testing.T) {
	m := newMasker(t)
	raw := Call{Seq: 9, Caller: "orders", Target: "ledger", Method: "GET", Path: "/v1/entries", Started: callStart, Status: 200, Outcome: OutcomeAnswered, Response: Message{Body: Body{Kind: BodyJSON, JSON: map[string]any{"new_password": plantedCredential}}}}
	if line := lineOf(t, raw); m.ValueScan().Mask(line) != line {
		t.Fatal("the value scan already refuses the line, so the case proves nothing")
	}
	got, ok := proveCleaned(m, raw)
	if ok || !got.Withheld || strings.Contains(lineOf(t, got), plantedCredential) {
		t.Fatalf("proof %v, call %+v", ok, got)
	}
	clean, ok := proveCleaned(m, Clean(m, raw))
	if !ok || clean.Withheld {
		t.Fatalf("canary: the cleaned call: %v %+v", ok, clean)
	}
	assertMasked(t, "the cleaned call", lineOf(t, clean), plantedCredential)
}

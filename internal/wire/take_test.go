package wire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var takeMutations = []porttest.Mutation{
	{"a stored line is the proven line", "write the decoded call instead of the result of Prove in addLine", "red: the planted value stays in calls.jsonl", "TestATakeMasksEveryLineAndStoresOffsetsThatReadTheCallBack"},
	{"an index holds the links of the cleaned calls", "link the stored calls so that a stub loses its span", "red: the stub of a call with a known span links to no step", "TestAStubKeepsTheStepLinkItsSpanGave"},
	{"unreadable value sources store nothing", "treat a Record error as an empty masker", "red: the stream is read and a record is stored", "TestUnreadableValueSourcesWithholdTheTakeWithoutReadingTheStream"},
	{"a stream without its end is not stored", "accept a stream that ended without an End line", "red: the take panics on the end line that never came", "TestAStreamThatNeverCompletesFailsAndStoresNothing"},
	{"the end line must count the calls", "drop the count comparison from complete", "red: a stream that lost a line is stored", "TestAStreamThatNeverCompletesFailsAndStoresNothing"},
	{"a broken stream is read once more", "return failed after the first broken attempt", "red: a stream that breaks once fails the take", "TestAStreamThatBreaksOnceIsReadAgainFromTheStart"},
	{"a refused take is lost and not retried", "treat ErrRecorderGone as a broken stream", "red: the state reads failed after two attempts", "TestARecorderThatRefusesTheTakeIsLostAfterOneAttempt"},
	{"a restarted recorder loses the suite take", "drop the restarted check from attempt", "red: the partial suite record is stored", "TestARecorderStartedAfterTheFirstStepLosesTheSuiteTakeButNotAnExecTake"},
	{"a stored take is not read again", "drop the stored check from Take", "red: the stream is read a second time", "TestATakeAlreadyStoredReturnsItsStatusWithoutReadingTheStream"},
	{"take names cannot leave the record directory", "drop the ValidTake check from Take", "red: a file lands outside the record", "TestATakeNameThatIsNotValidStoresNothing"},
}

func TestTakeMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "take_test.go", takeMutations)
}

type fakeStream struct {
	lines []string
	reads int
	err   error
	each  func(read int) ([]string, error)
}

func (f *fakeStream) stream(_ context.Context, out io.Writer) error {
	f.reads++
	lines, err := f.lines, f.err
	if f.each != nil {
		lines, err = f.each(f.reads)
	}
	for _, l := range lines {
		fmt.Fprintln(out, l)
	}
	return err
}

func callLine(t *testing.T, c Call) string { t.Helper(); return lineOf(t, c) }

func endLine(t *testing.T, end End) string {
	t.Helper()
	return encoded(t, Line{End: &end})
}

func plainCall(seq int64, text string) Call {
	started := callStart.Add(time.Duration(seq) * time.Second)
	return Call{
		Seq: seq, Caller: "orders", Target: "ledger", Port: 8081, Method: "POST", Path: "/v1/entries",
		Started: started, Finished: started.Add(5 * time.Millisecond), Status: 200, Outcome: OutcomeAnswered,
		Request: Message{Size: int64(len(text)), Body: Body{Kind: BodyText, Text: text}},
	}
}

func takeInput(t *testing.T, stream *fakeStream) TakeInput {
	t.Helper()
	return TakeInput{
		Store: OpenStore(t.TempDir()), Name: SuiteTake,
		Record: func() (*mask.Masker, error) { return recordMasker(t), nil },
		Stream: stream.stream,
	}
}

func TestATakeMasksEveryLineAndStoresOffsetsThatReadTheCallBack(t *testing.T) {
	stream := &fakeStream{}
	stream.lines = []string{callLine(t, plainCall(1, "first "+planted)), callLine(t, plainCall(2, "second")), endLine(t, End{Calls: 2, BodiesOmitted: 3, CallsDropped: 4})}
	in := takeInput(t, stream)
	in.PublicFields = []contract.WirePublicField{{Key: "token", Value: "tok-[0-9a-f]{8,64}"}}
	in.Routes = []contract.WireRoute{{Name: "ledger", Service: "ledger", Port: 8081}}
	in.Result = &contract.Result{Steps: []contract.ResultStep{{Flow: "orders", ID: "create", Status: contract.StepStatusFailed}}}
	status := Take(context.Background(), in)
	want := Status{Take: SuiteTake, State: StateTaken, Calls: 2, BodiesOmitted: 3, CallsDropped: 4}
	if status != want {
		t.Fatalf("status %+v, want %+v", status, want)
	}
	root := in.Store.Root()
	raw, err := os.ReadFile(filepath.Join(root, SuiteTake, CallsFile))
	if err != nil {
		t.Fatal(err)
	}
	assertMasked(t, "calls.jsonl", string(raw), planted)
	for _, c := range []struct {
		seq  int64
		text string
	}{{1, "first " + marker}, {2, "second"}} {
		call, head, err := in.Store.Call(SuiteTake, c.seq)
		if err != nil || call.Request.Body.Text != c.text || head.Method != "POST" || head.DurationMs != 5 {
			t.Fatalf("call %d: %+v head %+v: %v", c.seq, call, head, err)
		}
	}
	if _, _, err := in.Store.Call(SuiteTake, 9); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("an unknown call: %v", err)
	}
	index, err := in.Store.Index(SuiteTake)
	if err != nil || index.Status != want || len(index.PublicFields) != 1 || len(index.Routes) != 1 || index.Routes[0].Name != "ledger" || len(index.Steps) != 1 || index.Steps[0].Status != contract.StepStatusFailed {
		t.Fatalf("index %+v: %v", index, err)
	}
	for _, name := range []string{CallsFile, IndexFile} {
		info, err := os.Stat(filepath.Join(root, SuiteTake, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", name, info, err)
		}
	}
	info, err := os.Stat(filepath.Join(root, SuiteTake))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("take directory: %v %v", info, err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 || entries[0].Name() != SuiteTake {
		t.Fatalf("root holds %v", entries)
	}
}

func TestAStubKeepsTheStepLinkItsSpanGave(t *testing.T) {
	c := plainCall(1, pemBegin)
	c.Span = "00f067aa0ba902b7"
	c.Response = Message{Size: 1, Body: Body{Kind: BodyText, Text: pemEnd}}
	in := takeInput(t, &fakeStream{lines: []string{callLine(t, c), endLine(t, End{Calls: 1})}})
	in.Stream = (&fakeStream{lines: []string{callLine(t, c), endLine(t, End{Calls: 1})}}).stream
	in.Result = &contract.Result{Steps: []contract.ResultStep{{Flow: "orders", ID: "create", Calls: []contract.StepCall{{SpanID: c.Span}}}}}
	status := Take(context.Background(), in)
	if status.State != StateTaken || status.WithheldCalls != 1 {
		t.Fatalf("status %+v", status)
	}
	index, err := in.Store.Index(SuiteTake)
	if err != nil {
		t.Fatal(err)
	}
	stored, _, err := in.Store.Call(SuiteTake, 1)
	if err != nil || !stored.Withheld || stored.Span != "" {
		t.Fatalf("stored %+v: %v", stored, err)
	}
	if head := index.Heads[0]; head.Link.Kind != LinkExact || index.Steps[head.Link.Step] != (StepRef{Flow: "orders", Step: "create"}) {
		t.Fatalf("head %+v steps %+v", head, index.Steps)
	}
}

func TestUnreadableValueSourcesWithholdTheTakeWithoutReadingTheStream(t *testing.T) {
	stream := &fakeStream{lines: []string{endLine(t, End{})}}
	in := takeInput(t, stream)
	in.Record = func() (*mask.Masker, error) { return nil, errors.New("no sources") }
	status := Take(context.Background(), in)
	if status.State != StateWithheld || status.Detail == "" || stream.reads != 0 {
		t.Fatalf("status %+v reads %d", status, stream.reads)
	}
	if _, err := os.Stat(in.Store.Root()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the record directory exists: %v", err)
	}
}

func TestAStreamThatNeverCompletesFailsAndStoresNothing(t *testing.T) {
	cases := map[string][]string{
		"no end line":            {callLine(t, plainCall(1, "a"))},
		"an end that miscounts":  {callLine(t, plainCall(1, "a")), endLine(t, End{Calls: 2})},
		"a line after the end":   {endLine(t, End{}), callLine(t, plainCall(1, "a"))},
		"a line that is no JSON": {"{not json", endLine(t, End{})},
		"a line with neither":    {"{}", endLine(t, End{})},
	}
	executed := 0
	for name, lines := range cases {
		t.Run(name, func(t *testing.T) {
			stream := &fakeStream{lines: lines}
			in := takeInput(t, stream)
			status := Take(context.Background(), in)
			if status.State != StateFailed || status.Calls != 0 || stream.reads != takeAttempts {
				t.Fatalf("status %+v reads %d", status, stream.reads)
			}
			if takes, err := in.Store.Takes(); err != nil || len(takes) != 0 {
				t.Fatalf("takes %v: %v", takes, err)
			}
			entries, _ := os.ReadDir(in.Store.Root())
			if len(entries) != 0 {
				t.Fatalf("left %v", entries)
			}
		})
		executed++
	}
	if executed != 5 {
		t.Fatalf("executed %d", executed)
	}
}

func TestAStreamThatBreaksOnceIsReadAgainFromTheStart(t *testing.T) {
	stream := &fakeStream{}
	stream.each = func(read int) ([]string, error) {
		if read == 1 {
			return []string{callLine(t, plainCall(1, "partial"))}, errors.New("broken pipe")
		}
		return []string{callLine(t, plainCall(1, "whole")), callLine(t, plainCall(2, "two")), endLine(t, End{Calls: 2})}, nil
	}
	in := takeInput(t, stream)
	if status := Take(context.Background(), in); status.State != StateTaken || status.Calls != 2 || stream.reads != 2 {
		t.Fatalf("status %+v reads %d", status, stream.reads)
	}
	call, _, err := in.Store.Call(SuiteTake, 1)
	if err != nil || call.Request.Body.Text != "whole" {
		t.Fatalf("call %+v: %v", call, err)
	}
}

func TestARecorderThatRefusesTheTakeIsLostAfterOneAttempt(t *testing.T) {
	stream := &fakeStream{err: ErrRecorderGone}
	in := takeInput(t, stream)
	if status := Take(context.Background(), in); status.State != StateLost || stream.reads != 1 || status.Detail == "" {
		t.Fatalf("status %+v reads %d", status, stream.reads)
	}
	midway := &fakeStream{lines: []string{callLine(t, plainCall(1, "a"))}, err: ErrRecorderGone}
	if status := Take(context.Background(), takeInput(t, midway)); status.State != StateFailed || midway.reads != takeAttempts {
		t.Fatalf("a refusal after lines is a broken stream: %+v reads %d", status, midway.reads)
	}
}

func TestARecorderStartedAfterTheFirstStepLosesTheSuiteTakeButNotAnExecTake(t *testing.T) {
	first := callStart.Add(time.Minute)
	result := &contract.Result{Steps: []contract.ResultStep{
		{Flow: "f", ID: "later", StartedAt: first.Add(time.Hour).Format(time.RFC3339Nano)},
		{Flow: "f", ID: "first", StartedAt: first.Format(time.RFC3339Nano)},
	}}
	run := func(name string, since time.Time) Status {
		stream := &fakeStream{lines: []string{callLine(t, plainCall(1, "a")), endLine(t, End{Calls: 1, Since: since})}}
		in := takeInput(t, stream)
		in.Name, in.Result = name, result
		return Take(context.Background(), in)
	}
	if status := run(SuiteTake, first.Add(time.Second)); status.State != StateLost || status.Calls != 0 {
		t.Fatalf("restarted: %+v", status)
	}
	if status := run(SuiteTake, first.Add(-time.Second)); status.State != StateTaken {
		t.Fatalf("started before the steps: %+v", status)
	}
	if status := run("2026-exec-health", first.Add(time.Second)); status.State != StateTaken {
		t.Fatalf("an exec take: %+v", status)
	}
}

func TestATakeAlreadyStoredReturnsItsStatusWithoutReadingTheStream(t *testing.T) {
	stream := &fakeStream{lines: []string{callLine(t, plainCall(1, "a")), endLine(t, End{Calls: 1})}}
	in := takeInput(t, stream)
	first := Take(context.Background(), in)
	if first.State != StateTaken {
		t.Fatalf("first %+v", first)
	}
	in.Record = func() (*mask.Masker, error) { return nil, errors.New("unused") }
	if again := Take(context.Background(), in); again != first || stream.reads != 1 {
		t.Fatalf("again %+v reads %d", again, stream.reads)
	}
}

func TestATakeNameThatIsNotValidStoresNothing(t *testing.T) {
	names := []string{"../escape", ".hidden", "", "a/b", strings.Repeat("a", 129)}
	for _, name := range names {
		stream := &fakeStream{lines: []string{endLine(t, End{})}}
		in := takeInput(t, stream)
		in.Name = name
		if status := Take(context.Background(), in); status.State != StateFailed || stream.reads != 0 {
			t.Fatalf("%q: %+v reads %d", name, status, stream.reads)
		}
		if _, err := in.Store.Index(name); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("%q: index %v", name, err)
		}
	}
	if !ValidTake("2026-11-18T09-00-00-000Z-health") || !ValidTake(SuiteTake) {
		t.Fatal("canary: real take names are valid")
	}
	if len(names) != 5 {
		t.Fatalf("executed %d names", len(names))
	}
}

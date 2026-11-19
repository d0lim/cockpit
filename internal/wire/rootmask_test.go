package wire

import (
	"context"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var rootMaskMutations = []porttest.Mutation{
	{"a root name passes the record masker", "store link.Root without Mask in storedLink", "red: the planted value is in the index", "TestARootNameIsMaskedInTheIndexAndAbsentFromAWithheldCall"},
	{"a withheld call keeps no root name", "drop the Withheld check in storedLink", "red: the stub's head holds a root", "TestARootNameIsMaskedInTheIndexAndAbsentFromAWithheldCall"},
}

func TestRootMaskMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "rootmask_test.go", rootMaskMutations)
}

func TestARootNameIsMaskedInTheIndexAndAbsentFromAWithheldCall(t *testing.T) {
	clean, stub := plainCall(1, "a"), plainCall(2, pemBegin)
	clean.Span, stub.Span = clientSpan, serverSpan
	stub.Response = Message{Size: 1, Body: Body{Kind: BodyText, Text: pemEnd}}
	stream := &fakeStream{lines: []string{callLine(t, clean), callLine(t, stub), endLine(t, End{Calls: 2})}}
	in := takeInput(t, stream)
	in.Result = &contract.Result{}
	long := strings.Repeat("x", rootNameLimit*2)
	in.Spans = func() []Span {
		return []Span{span(clientSpan, "", "orders", "task "+planted+" "+long, 0), span(serverSpan, "", "orders", "task other", 0)}
	}
	if status := Take(context.Background(), in); status.State != StateTaken || status.WithheldCalls != 1 {
		t.Fatalf("status %+v", status)
	}
	index, err := in.Store.Index(SuiteTake)
	if err != nil {
		t.Fatal(err)
	}
	root := index.Heads[0].Link.Root
	if index.Heads[0].Link.Kind != LinkBackground || strings.Contains(root, planted) || !strings.Contains(root, "****") || len([]rune(root)) != rootNameLimit {
		t.Fatalf("root %q of %+v", root, index.Heads[0].Link)
	}
	if got := index.Heads[1].Link; got.Kind != LinkBackground || got.Root != "" {
		t.Fatalf("the withheld call: %+v", got)
	}
	if index.Heads[0].Finished.IsZero() {
		t.Fatalf("the head keeps no finish time: %+v", index.Heads[0])
	}
}

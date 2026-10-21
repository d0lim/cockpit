package signal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var frozenMutations = []porttest.Mutation{
	{"a frozen read masks again with the current value list", "append the stored line without remask in ReadFrozen", "red: a value added after the freeze shows in the read", "TestAFrozenReadMasksAgainAndPagesBySeq"},
	{"a bundle without a frozen copy reads as absent", "return an empty response when stream.jsonl is missing", "red: the error is not ErrNoFrozenCopy", "TestABundleWithoutAFrozenCopyIsAbsent"},
}

func TestFrozenMutationRecordMatchesTheTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "frozen_test.go", frozenMutations)
}

func TestAFrozenReadMasksAgainAndPagesBySeq(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	for _, body := range []string{"first", "second carries " + planted, "third"} {
		mustAppend(t, s, event("toy-db", body, contract.Attrs{"table": "orders"}))
	}
	bundle := filepath.Join(t.TempDir(), "report")
	if _, err := s.Freeze(bundle); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(bundle, Dir, streamFile))
	if !strings.Contains(string(raw), planted) {
		t.Fatal("canary: the copy was written before the value was known")
	}
	page, err := ReadFrozen(bundle, masker(t, planted), ReadRequest{After: 0, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	text := page.Signals[1].Event.Body
	if len(page.Signals) != 2 || strings.Contains(text, planted) || !strings.Contains(text, "****") || !page.Frozen || page.FrozenSeq != 3 || page.Next != 2 {
		t.Fatalf("first page %+v", page)
	}
	rest, err := ReadFrozen(bundle, masker(t, planted), ReadRequest{After: page.Next, Limit: 2})
	if err != nil || len(rest.Signals) != 1 || rest.Signals[0].Event.Body != "third" || rest.Next != 3 {
		t.Fatalf("second page %+v %v", rest, err)
	}
}

func TestABundleWithoutAFrozenCopyIsAbsent(t *testing.T) {
	bundle := t.TempDir()
	if _, err := ReadFrozen(bundle, masker(t), ReadRequest{}); !errors.Is(err, ErrNoFrozenCopy) {
		t.Fatalf("err %v", err)
	}
	os.MkdirAll(filepath.Join(bundle, Dir), 0o700)
	if _, err := ReadFrozen(bundle, masker(t), ReadRequest{}); !errors.Is(err, ErrNoFrozenCopy) {
		t.Fatalf("a signals directory without its stream: %v", err)
	}
	os.WriteFile(filepath.Join(bundle, Dir, streamFile), []byte(""), 0o600)
	if resp, err := ReadFrozen(bundle, masker(t), ReadRequest{}); err != nil || len(resp.Signals) != 0 || !resp.Frozen {
		t.Fatalf("canary: an empty copy reads as frozen and empty: %+v %v", resp, err)
	}
}

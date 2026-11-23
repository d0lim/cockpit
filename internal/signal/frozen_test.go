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
	{"a copy of the frozen lines masks each one again", "write the line as read in Remask", "red: a value added after the freeze shows in the copy", "TestRemaskCopiesEveryFrozenLineThroughTheMaskerAndRefusesABrokenOne"},
	{"a broken frozen line stops the copy", "skip a line that does not parse in Remask", "red: the copy ends without an error", "TestRemaskCopiesEveryFrozenLineThroughTheMaskerAndRefusesABrokenOne"},
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

func TestRemaskCopiesEveryFrozenLineThroughTheMaskerAndRefusesABrokenOne(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	for _, body := range []string{"first", "second carries " + planted, "third"} {
		mustAppend(t, s, event("toy-db", body, contract.Attrs{"table": "orders"}))
	}
	bundle := filepath.Join(t.TempDir(), "report")
	if _, err := s.Freeze(bundle); err != nil {
		t.Fatal(err)
	}
	raw := fileText(t, filepath.Join(bundle, FrozenPath))
	if !strings.Contains(raw, planted) {
		t.Fatal("canary: the copy was written before the value was known")
	}
	var same, masked strings.Builder
	if lines, err := Remask(masker(t), strings.NewReader(raw), &same); err != nil || lines != 3 || same.String() != raw {
		t.Fatalf("a copy with nothing new to mask: %d lines, %v\n%s", lines, err, same.String())
	}
	lines, err := Remask(masker(t, planted), strings.NewReader(raw), &masked)
	if err != nil || lines != 3 || strings.Contains(masked.String(), planted) || !strings.Contains(masked.String(), "second carries ****") || !strings.Contains(masked.String(), "third") {
		t.Fatalf("%d lines, %v\n%s", lines, err, masked.String())
	}
	if _, err := Remask(nil, strings.NewReader(raw), &strings.Builder{}); err == nil {
		t.Fatal("a copy without a masker went through")
	}
	if lines, err := Remask(masker(t), strings.NewReader(raw+"{not json\n"), &strings.Builder{}); err == nil || lines != 3 {
		t.Fatalf("a broken line: %d lines, %v", lines, err)
	}
}

package signal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var refusalMutations = []porttest.Mutation{
	{"reads of a run whose bundle was discarded are refused", "skip the ReadRefused check in Read", "red: the read returns signals", "TestReadOfARunWhoseBundleWasDiscardedIsRefusedAndStaysRefusedAfterRestart"},
	{"the refusal survives a restart", "keep the refusal in memory only", "red: the reopened stream reads", "TestReadOfARunWhoseBundleWasDiscardedIsRefusedAndStaysRefusedAfterRestart"},
	{"destroy removes the signal store", "Destroy only closes the file", "red: signals/ still exists", "TestDestroyRemovesTheSignalsDirectoryAndRemoveRunUsesTheRunLayout"},
}

func TestReadOfARunWhoseBundleWasDiscardedIsRefusedAndStaysRefusedAfterRestart(t *testing.T) {
	dir := runDir(t)
	m := masker(t, planted)
	s := openAt(t, dir, DefaultCaps(), m)
	mustAppend(t, s, event("toy-db", "row", nil))
	if got := readAll(t, s, 0); len(got.Signals) != 1 {
		t.Fatalf("canary: %+v", got)
	}
	if err := s.RefuseReads("bundle discarded: 1 file had unredacted values"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(ReadRequest{}); !errors.Is(err, ErrReadRefused) {
		t.Fatalf("read: %v", err)
	}
	s.Close()
	again := openAt(t, dir, DefaultCaps(), m)
	if _, err := again.Read(ReadRequest{}); !errors.Is(err, ErrReadRefused) {
		t.Fatalf("read after restart: %v", err)
	}
	if _, err := openAt(t, runDir(t), DefaultCaps(), m).Read(ReadRequest{}); err != nil {
		t.Fatalf("canary: an untouched run refuses reads: %v", err)
	}
}

func TestDestroyRemovesTheSignalsDirectoryAndRemoveRunUsesTheRunLayout(t *testing.T) {
	dir := runDir(t)
	s := openAt(t, dir, DefaultCaps(), masker(t))
	mustAppend(t, s, event("toy-db", "row", nil))
	if err := s.KVPut("toy-db", map[string]any{"cursor": 4.0}); err != nil {
		t.Fatal(err)
	}
	if err := s.Destroy(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, Dir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("signals/ left: %v", err)
	}
	st := store.Open(filepath.Join(t.TempDir(), "home"))
	id := "run-20261012-153002-997a37e"
	opened, err := OpenRun(st, id, Options{Masker: s.opts.Masker})
	if err != nil {
		t.Fatal(err)
	}
	opened.Close()
	runPath, _ := st.RunDir(id)
	if _, err := os.Stat(filepath.Join(runPath, Dir, streamFile)); err != nil {
		t.Fatalf("stream not under runs/<id>/signals: %v", err)
	}
	if err := RemoveRun(st, id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runPath, Dir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("RemoveRun left signals/: %v", err)
	}
	if err := RemoveRun(st, "../escape"); err == nil {
		t.Fatal("RemoveRun accepted a path that is not a run id")
	}
}

func TestRefusalMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "refusal_test.go", refusalMutations)
}

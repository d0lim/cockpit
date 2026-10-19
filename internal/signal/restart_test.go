package signal

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRestartRebuildsTheIndexFromTheFileAndCutsAnIncompleteLastLine(t *testing.T) {
	dir := runDir(t)
	m := masker(t)
	s := openAt(t, dir, DefaultCaps(), m)
	for range 3 {
		mustAppend(t, s, metric("toy-db", 1))
	}
	s.Close()
	path := filepath.Join(dir, Dir, streamFile)
	whole := fileText(t, path)
	f, _ := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	f.WriteString(`{"seq":4,"ts":"2026-10-12T07:00:01Z","kind":"met`)
	f.Close()
	again := openAt(t, dir, DefaultCaps(), m)
	if fileText(t, path) != whole {
		t.Fatal("the incomplete last line was not cut")
	}
	if got := readAll(t, again, 0); !slices.Equal(seqs(got), []int64{1, 2, 3}) {
		t.Fatalf("%v", seqs(got))
	}
	if seq := mustAppend(t, again, metric("toy-db", 2)); seq != 4 {
		t.Fatalf("next seq %d", seq)
	}
	if !strings.HasSuffix(fileText(t, path), "}\n") || strings.Count(fileText(t, path), "\n") != 4 {
		t.Fatalf("%q", fileText(t, path))
	}
}

func TestCorruptLineInTheMiddleFailsTheOpenInsteadOfDroppingData(t *testing.T) {
	dir := runDir(t)
	s := openAt(t, dir, DefaultCaps(), masker(t))
	mustAppend(t, s, metric("toy-db", 1))
	s.Close()
	path := filepath.Join(dir, Dir, streamFile)
	text := fileText(t, path)
	os.WriteFile(path, []byte("{broken\n"+text), 0o600)
	if _, err := Open(dir, Options{Masker: s.opts.Masker}); err == nil || !strings.Contains(err.Error(), "corrupt at offset 0") {
		t.Fatalf("err %v", err)
	}
}

func TestRestartKeepsTheFrozenBoundaryTailDropsAndCounters(t *testing.T) {
	dir := runDir(t)
	m := masker(t)
	c := caps(2, 0, 0)
	s := openAt(t, dir, c, m)
	mustAppend(t, s, metric("toy-db", 1))
	if _, err := s.Freeze(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		mustAppend(t, s, metric("toy-db", 2))
	}
	before := readAll(t, s, 0)
	s.Close()
	again := openAt(t, dir, c, m)
	after := readAll(t, again, 0)
	if !slices.Equal(seqs(after), seqs(before)) || !maps.Equal(after.Dropped, before.Dropped) || after.FrozenSeq != 1 || !after.Frozen {
		t.Fatalf("before %v %v, after %v %v", seqs(before), before.Dropped, seqs(after), after.Dropped)
	}
	if _, err := again.Freeze(t.TempDir()); err != ErrAlreadyFrozen {
		t.Fatalf("second freeze: %v", err)
	}
	mustAppend(t, again, metric("toy-db", 3))
	if got := readAll(t, again, 0); !slices.Equal(seqs(got), []int64{1, 5, 6}) || got.Dropped[ReasonTailCap] != 3 {
		t.Fatalf("%v %v", seqs(got), got.Dropped)
	}
}

func TestCompactionShrinksTheFileAndKeepsReadsCountersAndSeqsAcrossRestart(t *testing.T) {
	dir := runDir(t)
	m := masker(t)
	c := Caps{Tail: 2, CompactMin: 4}
	s := openAt(t, dir, c, m)
	mustAppend(t, s, metric("toy-db", 1))
	if _, err := s.Freeze(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		mustAppend(t, s, metric("toy-db", 2))
	}
	path := filepath.Join(dir, Dir, streamFile)
	if lines := strings.Count(fileText(t, path), "\n"); lines >= 11 {
		t.Fatalf("not compacted: %d lines", lines)
	}
	before := readAll(t, s, 0)
	if !slices.Equal(seqs(before), []int64{1, 10, 11}) || before.Dropped[ReasonTailCap] != 8 {
		t.Fatalf("%v %v", seqs(before), before.Dropped)
	}
	s.Close()
	again := openAt(t, dir, c, m)
	after := readAll(t, again, 0)
	if !slices.Equal(seqs(after), seqs(before)) || !maps.Equal(after.Dropped, before.Dropped) {
		t.Fatalf("%v %v", seqs(after), after.Dropped)
	}
	if seq := mustAppend(t, again, metric("toy-db", 3)); seq != 12 {
		t.Fatalf("seq went back to %d", seq)
	}
	if resync := readAll(t, again, 1).Resync; resync == nil || resync.From != 2 {
		t.Fatalf("compacted seqs do not resync: %+v", resync)
	}
}

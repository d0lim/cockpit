package signal

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var supersedeMutations = []porttest.Mutation{
	{"a superseded revision leaves the collector quota", "skip lim.release in record", "red: 30000 re-emissions of 10 ids drop collector-quota records", "TestThirtyThousandReemissionsOfTenIDsLeaveTenWinnersAndNoQuotaDrops"},
	{"a late lower revision never counts", "admit the losing revision instead of superseding it", "red: late revisions fill the quota", "TestThirtyThousandReemissionsOfTenIDsLeaveTenWinnersAndNoQuotaDrops"},
	{"superseded records are not a gap", "count superseded entries as a resync gap in Read", "red: the read starts with a resync marker", "TestThirtyThousandReemissionsOfTenIDsLeaveTenWinnersAndNoQuotaDrops"},
	{"the quota still bites for distinct ids", "set every bucket quota to zero", "red: 30000 distinct ids keep 30000 records", "TestThirtyThousandDistinctIDsStillHitTheCollectorQuota"},
	{"a frozen record is never superseded away", "drop the inFrozenSegment check in release", "red: the frozen winner disappears from the stream file", "TestARevisionOfAFrozenWinnerLeavesTheFrozenRecordAndTheFrozenCopyHoldsOnlyWinners"},
	{"compaction keeps superseded ranges as tombstones for restart", "write nothing for superseded entries in maybeCompact", "red: after restart the superseded seqs read as part of the resync gap", "TestRestartAfterCompactionReplaysTheSameWinnersCountersAndNextSeq"},
	{"compaction weighs killed records against live ones", "compare against len(entries)-killed, which counts earlier removed seqs as retained", "red: compaction stops after the first rewrite and the file keeps over 1000 lines", "TestRestartAfterCompactionReplaysTheSameWinnersCountersAndNextSeq"},
}

func TestSupersedeMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "supersede_test.go", supersedeMutations)
}

func snapshot(i, round int) contract.Signal {
	return withID(event("toy-db", fmt.Sprintf("row %d round %d", i, round), contract.Attrs{"table": "items"}), fmt.Sprintf("item-%d", i), round)
}

func bodies(resp ReadResponse) []string {
	out := []string{}
	for _, sig := range resp.Signals {
		if sig.Event != nil {
			out = append(out, sig.Event.Body)
		}
	}
	slices.Sort(out)
	return out
}

func TestThirtyThousandReemissionsOfTenIDsLeaveTenWinnersAndNoQuotaDrops(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	for round := 1; round <= 3000; round++ {
		for i := range 10 {
			mustAppend(t, s, snapshot(i, round))
		}
	}
	for i := range 10 {
		mustAppend(t, s, snapshot(i, 1))
	}
	got := readAll(t, s, 0)
	if len(got.Signals) != 10 || got.Resync != nil || len(got.Dropped) != 0 || got.Next != 30010 {
		t.Fatalf("%d signals, resync %+v, dropped %v, next %d", len(got.Signals), got.Resync, got.Dropped, got.Next)
	}
	for _, b := range bodies(got) {
		if !strings.HasSuffix(b, "round 3000") {
			t.Fatalf("a non-winner was read: %s", b)
		}
	}
	if seq, n := s.Revisions("toy-db", contract.SignalKindEvent, "item-3"); seq == 0 || n != 3001 {
		t.Fatalf("revisions of item-3: %d %d", seq, n)
	}
}

func TestThirtyThousandDistinctIDsStillHitTheCollectorQuota(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	for i := range 30000 {
		mustAppend(t, s, withID(event("toy-db", "row", nil), fmt.Sprintf("item-%d", i), 1))
	}
	got, err := s.Read(ReadRequest{Limit: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if got.Dropped[ReasonCollectorQuota] != 10000 || len(got.Signals) != DefaultPerCollector || got.Resync == nil {
		t.Fatalf("dropped %v, %d signals, resync %+v", got.Dropped, len(got.Signals), got.Resync)
	}
}

func TestARevisionOfAFrozenWinnerLeavesTheFrozenRecordAndTheFrozenCopyHoldsOnlyWinners(t *testing.T) {
	dir := runDir(t)
	s := openAt(t, dir, DefaultCaps(), masker(t))
	mustAppend(t, s, withID(event("toy-db", "pending", nil), "order-7", 1))
	frozenWinner := mustAppend(t, s, withID(event("toy-db", "paid", nil), "order-7", 2))
	bundle := filepath.Join(t.TempDir(), "report.partial")
	if _, err := s.Freeze(bundle); err != nil {
		t.Fatal(err)
	}
	copied := fileText(t, filepath.Join(bundle, Dir, streamFile))
	if strings.Count(copied, "\n") != 1 || !strings.Contains(copied, "paid") || strings.Contains(copied, "pending") {
		t.Fatalf("frozen copy %q", copied)
	}
	mustAppend(t, s, withID(event("toy-db", "shipped", nil), "order-7", 3))
	if !s.ix.alive(frozenWinner) || !strings.Contains(fileText(t, filepath.Join(dir, Dir, streamFile)), "paid") {
		t.Fatal("the frozen winner was removed")
	}
	if got := bodies(readAll(t, s, 0)); !slices.Equal(got, []string{"shipped"}) {
		t.Fatalf("read %v", got)
	}
}

func restartCase(t *testing.T, perCollector int) (ReadResponse, ReadResponse, *Stream) {
	t.Helper()
	dir := runDir(t)
	m := masker(t)
	c := Caps{Tail: DefaultTail, PerCollector: perCollector, PerContainer: DefaultPerContainer, CompactMin: 100}
	s := openAt(t, dir, c, m)
	for round := 1; round <= 300; round++ {
		for i := range 10 {
			mustAppend(t, s, snapshot(i, round))
		}
		mustAppend(t, s, metric("toy-db", float64(round)))
	}
	before := readAll(t, s, 0)
	if strings.Count(fileText(t, filepath.Join(dir, Dir, streamFile)), "\n") > 1000 {
		t.Fatal("canary: compaction never ran")
	}
	s.Close()
	again := openAt(t, dir, c, m)
	return before, readAll(t, again, 0), again
}

func TestRestartAfterCompactionReplaysTheSameWinnersCountersAndNextSeq(t *testing.T) {
	before, after, again := restartCase(t, DefaultPerCollector)
	if !slices.Equal(seqs(before), seqs(after)) || len(after.Dropped) != 0 || before.Resync != nil || after.Resync != nil || len(bodies(after)) != 10 {
		t.Fatalf("no drops: before %v %+v, after %v %+v %v", seqs(before), before.Resync, seqs(after), after.Resync, after.Dropped)
	}
	if seq := mustAppend(t, again, snapshot(0, 301)); seq != 3301 {
		t.Fatalf("next seq %d", seq)
	}
	if got := readAll(t, again, 0); len(bodies(got)) != 10 || !slices.Contains(bodies(got), "row 0 round 301") {
		t.Fatalf("after one more revision: %v", bodies(got))
	}
	before, after, _ = restartCase(t, 50)
	if !slices.Equal(seqs(before), seqs(after)) || !maps.Equal(before.Dropped, after.Dropped) || before.Dropped[ReasonCollectorQuota] == 0 ||
		before.Resync == nil || after.Resync == nil || *before.Resync != *after.Resync {
		t.Fatalf("with drops: before %v %+v, after %v %+v", before.Dropped, before.Resync, after.Dropped, after.Resync)
	}
}

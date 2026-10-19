package signal

import (
	"runtime"
	"testing"

	"github.com/d0lim/cockpit/internal/mask"
)

const (
	benchRecords   = 300_000
	testRecords    = 10_000
	maxRecordBytes = 16
)

func heap() uint64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

func BenchmarkIndexMemoryAfterThreeHundredThousandAppends(b *testing.B) {
	dir := runDir(b)
	m := masker(b)
	c := Caps{Tail: benchRecords * 2, PerCollector: benchRecords * 2, PerContainer: benchRecords * 2, CompactMin: benchRecords}
	s := openAt(b, dir, c, m)
	for range benchRecords {
		mustAppend(b, s, metric("toy-db", 1))
	}
	s.Close()
	b.ResetTimer()
	for b.Loop() {
		before := heap()
		again, err := Open(dir, Options{Caps: c, Masker: func() *mask.Masker { return m }})
		if err != nil {
			b.Fatal(err)
		}
		after := heap()
		b.ReportMetric(float64(after-before)/benchRecords, "heap-bytes/record")
		b.ReportMetric(float64(cap(again.ix.entries)*8)/benchRecords, "index-bytes/record")
		again.Close()
	}
}

func TestTheReopenedIndexHoldsAtMostSixteenBytesPerRecord(t *testing.T) {
	dir := runDir(t)
	m := masker(t)
	c := Caps{Tail: testRecords * 2, PerCollector: testRecords * 2, PerContainer: testRecords * 2, CompactMin: testRecords}
	s := openAt(t, dir, c, m)
	for range testRecords {
		mustAppend(t, s, metric("toy-db", 1))
	}
	s.Close()
	before := heap()
	again, err := Open(dir, Options{Caps: c, Masker: func() *mask.Masker { return m }})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	grown := float64(int64(heap())-int64(before)) / testRecords
	index := float64(cap(again.ix.entries)*8) / testRecords
	if len(again.ix.entries) < testRecords || index > maxRecordBytes || grown > maxRecordBytes {
		t.Fatalf("%d entries, index %.2f bytes/record, heap growth %.2f bytes/record, bound %d", len(again.ix.entries), index, grown, maxRecordBytes)
	}
}

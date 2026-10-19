package signal

import (
	"runtime"
	"testing"

	"github.com/d0lim/cockpit/internal/mask"
)

const benchRecords = 300_000

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

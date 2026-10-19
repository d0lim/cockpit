package signal

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

func caps(tail, perCollector, perContainer int) Caps {
	return Caps{Tail: tail, PerCollector: perCollector, PerContainer: perContainer, CompactMin: 1 << 30}
}

func span1to(n int64) []int64 {
	out := []int64{}
	for i := int64(1); i <= n; i++ {
		out = append(out, i)
	}
	return out
}

func TestOverflowingTheTailDropsOnlyTheOldestTailRecordAndLeavesTheFrozenSegment(t *testing.T) {
	s := openAt(t, runDir(t), caps(3, 0, 0), masker(t))
	for range 4 {
		mustAppend(t, s, metric("toy-db", 1))
	}
	frozen, err := s.Freeze(t.TempDir())
	if err != nil || frozen.FrozenSeq != 4 || frozen.Records != 4 {
		t.Fatalf("%+v %v", frozen, err)
	}
	for range 3 {
		mustAppend(t, s, metric("toy-db", 2))
	}
	if got := readAll(t, s, 0); !slices.Equal(seqs(got), span1to(7)) || len(got.Dropped) != 0 {
		t.Fatalf("within the cap: %v %v", seqs(got), got.Dropped)
	}
	mustAppend(t, s, metric("toy-db", 3))
	got := readAll(t, s, 0)
	if !slices.Equal(seqs(got), []int64{1, 2, 3, 4, 6, 7, 8}) {
		t.Fatalf("%v", seqs(got))
	}
	if !maps.Equal(got.Dropped, map[string]int64{ReasonTailCap: 1}) || got.FrozenSeq != 4 || !got.Frozen {
		t.Fatalf("%+v", got)
	}
}

func TestReadAfterADroppedSeqReturnsAResyncMarkerAndTheNextRetainedSignals(t *testing.T) {
	s := openAt(t, runDir(t), caps(2, 0, 0), masker(t))
	mustAppend(t, s, metric("toy-db", 1))
	if _, err := s.Freeze(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		mustAppend(t, s, metric("toy-db", 2))
	}
	got := readAll(t, s, 1)
	if got.Resync == nil || *got.Resync != (Resync{From: 2, To: 3}) || !slices.Equal(seqs(got), []int64{4, 5}) {
		t.Fatalf("%+v %v", got.Resync, seqs(got))
	}
	if got.Dropped[ReasonTailCap] != 2 || got.FrozenSeq != 1 {
		t.Fatalf("%+v", got)
	}
	if fine := readAll(t, s, 3); fine.Resync != nil || !slices.Equal(seqs(fine), []int64{4, 5}) {
		t.Fatalf("canary: %+v", fine)
	}
}

func TestLogLinesOverAContainerQuotaDropOnlyFromThatContainer(t *testing.T) {
	s := openAt(t, runDir(t), caps(0, 0, 2), masker(t))
	for n := range 4 {
		mustAppend(t, s, logLine("app", n))
	}
	mustAppend(t, s, logLine("cache", 0))
	mustAppend(t, s, logLine("cache", 1))
	mustAppend(t, s, metric("toy-db", 1))
	got := readAll(t, s, 0)
	var kept []string
	for _, sig := range got.Signals {
		if sig.Event != nil {
			kept = append(kept, sig.Attrs[ContainerAttr].(string)+" "+sig.Event.Body)
		}
	}
	if !slices.Equal(kept, []string{"app line 2", "app line 3", "cache line 0", "cache line 1"}) {
		t.Fatalf("%v", kept)
	}
	if !maps.Equal(got.Dropped, map[string]int64{ReasonContainerQuota: 2}) || len(got.Signals) != 5 {
		t.Fatalf("%+v", got)
	}
}

func TestCollectorOverItsQuotaDropsItsOwnOldestRecordsOnly(t *testing.T) {
	s := openAt(t, runDir(t), caps(0, 2, 0), masker(t))
	for n := range 3 {
		mustAppend(t, s, metric("toy-db", float64(n)))
	}
	mustAppend(t, s, metric("acme-queue", 9))
	got := readAll(t, s, 0)
	if !slices.Equal(seqs(got), []int64{2, 3, 4}) || got.Dropped[ReasonCollectorQuota] != 1 {
		t.Fatalf("%v %v", seqs(got), got.Dropped)
	}
}

func TestReadPagesFollowSeqOrderAndByTimeOrdersByInstantThenSeq(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	for range 5 {
		mustAppend(t, s, metric("toy-db", 1))
	}
	page, err := s.Read(ReadRequest{After: 1, Limit: 2})
	if err != nil || !slices.Equal(seqs(page), []int64{2, 3}) || page.Next != 3 {
		t.Fatalf("%v %d %v", seqs(page), page.Next, err)
	}
	signals := []contract.Signal{
		{Seq: 3, TS: "2026-10-12T07:00:00.5Z"},
		{Seq: 1, TS: "2026-10-12T07:00:00.500000Z"},
		{Seq: 2, TS: "2026-10-12T07:00:00Z"},
	}
	slices.SortStableFunc(signals, ByTime)
	if signals[0].Seq != 2 || signals[1].Seq != 1 || signals[2].Seq != 3 {
		t.Fatalf("%+v", signals)
	}
}

func TestSignalsWithABadCollectorKindBodyOrTimeSourceAreRefused(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	bad := []contract.Signal{
		event("../x", "", nil),
		event("Toy", "", nil),
		{Kind: contract.SignalKindEvent, Collector: "toy-db"},
		{Kind: contract.SignalKindMetric, Collector: "toy-db", Metric: &contract.MetricBody{}, Event: &contract.EventBody{}},
		{Kind: "trace", Collector: "toy-db", Event: &contract.EventBody{}},
		{Kind: contract.SignalKindEvent, Collector: "toy-db", TSSource: "wall", Event: &contract.EventBody{}},
		{Kind: contract.SignalKindSpan, Collector: "toy-db", Span: &contract.SpanBody{Status: "done"}},
	}
	for i, sig := range bad {
		if _, err := s.Append(sig); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
	if seq := mustAppend(t, s, event("toy-db", "ok", nil)); seq != 1 {
		t.Fatalf("canary: refused signals took seqs, got %d", seq)
	}
	if text := fileText(t, filepath.Join(s.dir, streamFile)); strings.Count(text, "\n") != 1 {
		t.Fatalf("%q", text)
	}
}

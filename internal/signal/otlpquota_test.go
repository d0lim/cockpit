package signal

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

func otlpSpan(service string, n int) contract.Signal {
	return contract.Signal{
		Kind: contract.SignalKindSpan, Collector: OTLPCollector, TSSource: contract.TimeSourceContainer,
		ID: fmt.Sprintf("w3c:%016x", n), Attrs: contract.Attrs{ServiceAttr: service},
		Span: &contract.SpanBody{TraceID: "w3c:" + fmt.Sprintf("%032x", 1), Name: "GET /", Start: "2026-10-12T07:00:01Z", End: "2026-10-12T07:00:02Z", Status: contract.SpanStatusOK},
	}
}

func serviceQuotaCaps() Caps {
	return Caps{Tail: 0, PerCollector: 2, PerContainer: 0, PerService: 3, CompactMin: 1 << 30}
}

func keptByService(got ReadResponse) map[string][]string {
	out := map[string][]string{}
	for _, sig := range got.Signals {
		key := sig.Collector
		if sig.Collector == OTLPCollector {
			key = sig.Attrs[ServiceAttr].(string)
		}
		out[key] = append(out[key], sig.ID)
	}
	return out
}

func appendServiceMix(t *testing.T, s *Stream) {
	for n := range 4 {
		mustAppend(t, s, otlpSpan("app", n))
	}
	mustAppend(t, s, otlpSpan("worker", 10))
	mustAppend(t, s, otlpSpan("worker", 11))
	for n := range 3 {
		mustAppend(t, s, metric("toy-db", float64(n)))
	}
}

func TestOTLPRecordsCountAgainstTheirServiceBucket(t *testing.T) {
	s := openAt(t, runDir(t), serviceQuotaCaps(), masker(t))
	appendServiceMix(t, s)
	got := readAll(t, s, 0)
	kept := keptByService(got)
	if !slices.Equal(kept["app"], []string{"w3c:0000000000000001", "w3c:0000000000000002", "w3c:0000000000000003"}) {
		t.Fatalf("app kept %v", kept["app"])
	}
	if len(kept["worker"]) != 2 || len(kept["toy-db"]) != 2 {
		t.Fatalf("canary: worker kept %v and toy-db kept %v", kept["worker"], kept["toy-db"])
	}
	if !maps.Equal(got.Dropped, map[string]int64{ReasonOTLPQuota: 1, ReasonCollectorQuota: 1}) {
		t.Fatalf("dropped %v", got.Dropped)
	}
	if DefaultCaps().PerService != 20_000 {
		t.Fatalf("the default service quota is %d", DefaultCaps().PerService)
	}
}

func TestOTLPServiceBucketsReplayAfterARestart(t *testing.T) {
	dir, m := runDir(t), masker(t)
	s := openAt(t, dir, serviceQuotaCaps(), m)
	appendServiceMix(t, s)
	before := readAll(t, s, 0)
	s.Close()
	again := openAt(t, dir, serviceQuotaCaps(), m)
	after := readAll(t, again, 0)
	if !slices.Equal(seqs(after), seqs(before)) || !maps.Equal(after.Dropped, before.Dropped) {
		t.Fatalf("before %v %v, after %v %v", seqs(before), before.Dropped, seqs(after), after.Dropped)
	}
	mustAppend(t, again, otlpSpan("app", 4))
	got := readAll(t, again, 0)
	if kept := keptByService(got); !slices.Equal(kept["app"], []string{"w3c:0000000000000002", "w3c:0000000000000003", "w3c:0000000000000004"}) || len(kept["worker"]) != 2 {
		t.Fatalf("after the restart app kept %v and worker kept %v", kept["app"], kept["worker"])
	}
	if got.Dropped[ReasonOTLPQuota] != 2 || got.Dropped[ReasonCollectorQuota] != 1 {
		t.Fatalf("dropped %v", got.Dropped)
	}
}

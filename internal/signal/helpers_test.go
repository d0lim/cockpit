package signal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
)

const planted = "PLANTED-row-secret-7f3a9c"

func masker(t testing.TB, known ...string) *mask.Masker {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: known})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func clock() func() time.Time {
	at := time.Date(2026, 10, 12, 7, 0, 0, 0, time.UTC)
	return func() time.Time { at = at.Add(time.Millisecond); return at }
}

func openAt(t testing.TB, dir string, caps Caps, m *mask.Masker) *Stream {
	t.Helper()
	s, err := Open(dir, Options{Caps: caps, Masker: func() *mask.Masker { return m }, Now: clock()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func runDir(t testing.TB) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "runs", "run-20261012-153001-997a37e")
}

func event(collector, body string, attrs contract.Attrs) contract.Signal {
	return contract.Signal{Kind: contract.SignalKindEvent, Collector: collector, Attrs: attrs, Event: &contract.EventBody{Body: body}}
}

func logLine(container string, n int) contract.Signal {
	return contract.Signal{Kind: contract.SignalKindEvent, Collector: LogCollector, TSSource: contract.TimeSourceContainer, Attrs: contract.Attrs{ContainerAttr: container}, Event: &contract.EventBody{Body: fmt.Sprintf("line %d", n)}}
}

func metric(collector string, v float64) contract.Signal {
	return contract.Signal{Kind: contract.SignalKindMetric, Collector: collector, Metric: &contract.MetricBody{Name: "rows", Value: v}}
}

func mustAppend(t testing.TB, s *Stream, sig contract.Signal) int64 {
	t.Helper()
	seq, err := s.Append(sig)
	if err != nil {
		t.Fatal(err)
	}
	return seq
}

func readAll(t testing.TB, s *Stream, after int64) ReadResponse {
	t.Helper()
	resp, err := s.Read(ReadRequest{After: after, Limit: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func seqs(resp ReadResponse) []int64 {
	out := []int64{}
	for _, sig := range resp.Signals {
		out = append(out, sig.Seq)
	}
	return out
}

func fileText(t testing.TB, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

package sse

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/signal"
)

type frame struct {
	id, event, data string
}

type fakeSignals struct {
	mu      sync.Mutex
	signals []contract.Signal
	resync  *signal.Resync
	refused bool
}

func (f *fakeSignals) Read(_ string, req signal.ReadRequest) (signal.ReadResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refused {
		return signal.ReadResponse{}, signal.ErrReadRefused
	}
	resp := signal.ReadResponse{Signals: []contract.Signal{}, Dropped: map[string]int64{}, Next: req.After}
	if f.resync != nil && req.After < f.resync.To {
		resp.Resync, resp.Next = f.resync, f.resync.To
	}
	for _, s := range f.signals {
		if s.Seq > resp.Next && len(resp.Signals) < req.Limit {
			resp.Signals = append(resp.Signals, s)
		}
	}
	if n := len(resp.Signals); n > 0 {
		resp.Next = resp.Signals[n-1].Seq
	}
	return resp, nil
}

func sig(seq int64) contract.Signal {
	return contract.Signal{Seq: seq, TS: "2026-10-19T01:00:00.000Z", TSSource: contract.TimeSourceHost, Kind: contract.SignalKindEvent, Collector: "toy-db", Attrs: contract.Attrs{}, Event: &contract.EventBody{Body: "row"}}
}

func serve(t *testing.T, s *Stream) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("target")
		s.Serve(w, r, target, target != HubTarget)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func collect(t *testing.T, srv *httptest.Server, target, lastID string, want int) []frame {
	return collectWithin(t, srv, target, lastID, want, 3*time.Second)
}

func collectWithin(t *testing.T, srv *httptest.Server, target, lastID string, want int, limit time.Duration) []frame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"?target="+target, nil)
	if lastID != "" {
		req.Header.Set(LastEventID, lastID)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.Header.Get("Content-Type") != contentType {
		t.Fatalf("content type %q", res.Header.Get("Content-Type"))
	}
	var out []frame
	var cur frame
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for len(out) < want && sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "id: "):
			cur.id = line[4:]
		case strings.HasPrefix(line, "event: "):
			cur.event = line[7:]
		case strings.HasPrefix(line, "data: "):
			cur.data = line[6:]
		case line == "" && cur.event != "":
			out = append(out, cur)
			cur = frame{}
		case strings.HasPrefix(line, ": ping"):
			out = append(out, frame{event: "ping"})
		}
	}
	return out
}

func signalSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "schema", "signal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	r, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAStreamReplaysPastEventsInSeqOrderThenSignalsWithCompositeIDs(t *testing.T) {
	b := New("1700", 0, nil)
	b.Publish(KindJob, runA, map[string]any{"text": "one"})
	b.Publish(KindRuns, HubTarget, map[string]any{"list": map[string]any{}})
	signals := &fakeSignals{signals: []contract.Signal{sig(1), sig(2)}}
	srv := serve(t, &Stream{Bus: b, Signals: signals, Poll: 10 * time.Millisecond, Ping: time.Hour})
	frames := collect(t, srv, runA, "", 4)
	want := []frame{{id: "1700.1.0", event: "job"}, {id: "1700.2.0", event: "runs"}, {id: "1700.2.1", event: "signal"}, {id: "1700.2.2", event: "signal"}}
	for i, w := range want {
		if frames[i].id != w.id || frames[i].event != w.event {
			t.Fatalf("frame %d: %+v, want %+v", i, frames[i], w)
		}
	}
	var job map[string]any
	json.Unmarshal([]byte(frames[0].data), &job)
	if job["kind"] != "job" || job["target"] != runA || job["text"] != "one" || job["at"] == nil {
		t.Fatalf("job data %v", job)
	}
	schema := signalSchema(t)
	for _, f := range frames[2:] {
		var doc map[string]any
		json.Unmarshal([]byte(f.data), &doc)
		if err := schema.Validate(doc); err != nil {
			t.Fatalf("signal frame does not pass schema/signal.json: %v\n%s", err, f.data)
		}
	}
	bad := map[string]any{"seq": 1, "kind": "event", "collector": "toy-db", "extra": true}
	if schema.Validate(bad) == nil {
		t.Fatal("canary: the schema accepts a malformed signal")
	}
}

func TestLastEventIDResumesAfterTheBusAndSignalSeqOfTheSameEpoch(t *testing.T) {
	b := New("1700", 0, nil)
	b.Publish(KindJob, runA, nil)
	b.Publish(KindJob, runA, nil)
	b.Publish(KindLog, runA, nil)
	signals := &fakeSignals{signals: []contract.Signal{sig(1), sig(2), sig(3)}}
	srv := serve(t, &Stream{Bus: b, Signals: signals, Poll: 10 * time.Millisecond, Ping: time.Hour})
	frames := collect(t, srv, runA, "1700.2.2", 2)
	if frames[0].id != "1700.3.2" || frames[0].event != "log" || frames[1].id != "1700.3.3" || frames[1].event != "signal" {
		t.Fatalf("frames %+v", frames)
	}
}

func TestAfterARestartAnOldLastEventIDOfAnotherEpochGetsEverythingFromTheStart(t *testing.T) {
	old := New("1700", 0, nil)
	for range 50 {
		old.Publish(KindLog, runA, nil)
	}
	restarted := New("1800", 0, nil)
	restarted.Publish(KindJob, runA, map[string]any{"text": "after restart"})
	signals := &fakeSignals{signals: []contract.Signal{sig(1)}}
	srv := serve(t, &Stream{Bus: restarted, Signals: signals, Poll: 10 * time.Millisecond, Ping: time.Hour})
	frames := collect(t, srv, runA, "1700.50.40", 2)
	if len(frames) != 2 {
		t.Fatalf("frames after restart %+v", frames)
	}
	if frames[0].id != "1800.1.0" || frames[0].event != "job" || frames[1].id != "1800.1.1" {
		t.Fatalf("frames %+v", frames)
	}
	same := collectWithin(t, srv, runA, "1800.1.1", 1, 300*time.Millisecond)
	if len(same) != 0 {
		t.Fatalf("canary: the same epoch resent %+v", same)
	}
}

func TestAResyncMarkerAndARefusedStreamAreFramesOfTheirOwn(t *testing.T) {
	b := New("1700", 0, nil)
	signals := &fakeSignals{signals: []contract.Signal{sig(4)}, resync: &signal.Resync{From: 1, To: 3}}
	srv := serve(t, &Stream{Bus: b, Signals: signals, Poll: 10 * time.Millisecond, Ping: time.Hour})
	frames := collect(t, srv, runA, "", 2)
	if frames[0].event != string(KindResync) || frames[0].id != "1700.0.3" || frames[1].id != "1700.0.4" {
		t.Fatalf("frames %+v", frames)
	}
	signals.mu.Lock()
	signals.refused = true
	signals.mu.Unlock()
	refused := collect(t, srv, runA, "", 1)
	if refused[0].event != string(KindRefused) || !strings.Contains(refused[0].data, "refused") {
		t.Fatalf("refused %+v", refused)
	}
}

func TestAnIdleStreamSendsAPingCommentAndTheHubTargetReadsNoSignals(t *testing.T) {
	b := New("1700", 0, nil)
	signals := &fakeSignals{signals: []contract.Signal{sig(1)}}
	srv := serve(t, &Stream{Bus: b, Signals: signals, Poll: 10 * time.Millisecond, Ping: 30 * time.Millisecond})
	frames := collect(t, srv, HubTarget, "", 1)
	if len(frames) != 1 || frames[0].event != "ping" {
		t.Fatalf("frames %+v", frames)
	}
	if DefaultPing != 15*time.Second {
		t.Fatalf("default ping %v", DefaultPing)
	}
}

func TestCursorsRoundTripAndMalformedIDsStartOver(t *testing.T) {
	c := Cursor{Epoch: "1700", Bus: 12, Signal: 40}
	if got, ok := ParseCursor(c.String()); !ok || got != c {
		t.Fatalf("round trip %+v", got)
	}
	for _, raw := range []string{"", "1700", "1700.x.1", "1700.1.-1", ".1.1", "1700.1.1.1"} {
		if _, ok := ParseCursor(raw); ok {
			t.Errorf("%q parsed", raw)
		}
	}
}

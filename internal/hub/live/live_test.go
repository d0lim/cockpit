package live

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/sse"
	"github.com/d0lim/cockpit/internal/signal"
)

const runA = "run-20261020-101500-abc1234"
const runB = "run-20261020-101600-def5678"

type fakeSignals struct {
	mu      sync.Mutex
	last    int64
	refused bool
}

func (f *fakeSignals) add(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.last += int64(n)
}

func (f *fakeSignals) refuse() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refused = true
}

func (f *fakeSignals) Read(_ string, req signal.ReadRequest) (signal.ReadResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refused {
		return signal.ReadResponse{}, signal.ErrReadRefused
	}
	resp := signal.ReadResponse{Signals: []contract.Signal{}, Next: req.After}
	for seq := req.After + 1; seq <= f.last && len(resp.Signals) < req.Limit; seq++ {
		resp.Signals = append(resp.Signals, contract.Signal{Seq: seq})
		resp.Next = seq
	}
	return resp, nil
}

type world struct {
	bus     *sse.Bus
	signals *fakeSignals
	mu      sync.Mutex
	gone    bool
	url     string
}

func (w *world) setGone() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.gone = true
}

func (w *world) resolve(_ context.Context, path string) (*Screen, error) {
	render := func(_ context.Context, regions []string) ([]byte, error) {
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.gone {
			return nil, ErrGone
		}
		return []byte(strings.Join(regions, ",")), nil
	}
	switch path {
	case "/":
		return &Screen{Regions: []string{"rail", "runs"}, Touches: map[sse.Kind][]string{sse.KindRuns: {"rail", "runs"}}, Render: render}, nil
	case "/runs/" + runA + "/pipeline":
		return &Screen{Run: runA, Regions: []string{"rail", "run-head", "pipeline"}, Touches: map[sse.Kind][]string{sse.KindRuns: {"rail", "run-head"}, sse.KindJob: {"pipeline"}}, Render: render}, nil
	case "/runs/" + runA + "/signals/events":
		return &Screen{Run: runA, Regions: []string{"rail", "run-head", "signals"}, Touches: map[sse.Kind][]string{sse.KindRuns: {"rail", "run-head"}}, Signals: []string{"signals"}, Render: render}, nil
	case "/hub":
		return nil, nil
	}
	return nil, errors.New("no such screen")
}

func setup(t *testing.T) *world {
	t.Helper()
	w := &world{bus: sse.New("1", 0, nil), signals: &fakeSignals{}}
	h := &Handler{Bus: w.bus, Signals: w.signals, Resolve: w.resolve, Flush: 20 * time.Millisecond, Poll: 10 * time.Millisecond, Ping: time.Hour}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	w.url = "ws" + strings.TrimPrefix(srv.URL, "http")
	return w
}

type client struct {
	msgs   chan string
	closed chan error
}

func (w *world) dial(t *testing.T, screen string) *client {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(w.url+"/live?screen="+screen, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	c := &client{msgs: make(chan string, 64), closed: make(chan error, 1)}
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				c.closed <- err
				return
			}
			c.msgs <- string(msg)
		}
	}()
	return c
}

func next(t *testing.T, c *client) string {
	t.Helper()
	select {
	case msg := <-c.msgs:
		return msg
	case err := <-c.closed:
		t.Fatalf("closed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("no message in 3s")
	}
	return ""
}

func quiet(t *testing.T, c *client, d time.Duration) {
	t.Helper()
	select {
	case msg := <-c.msgs:
		t.Fatalf("unexpected message %q", msg)
	case err := <-c.closed:
		t.Fatalf("closed: %v", err)
	case <-time.After(d):
	}
}

func closedNormally(t *testing.T, c *client) {
	t.Helper()
	select {
	case msg := <-c.msgs:
		t.Fatalf("message %q instead of a close", msg)
	case err := <-c.closed:
		if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
			t.Fatalf("close: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("not closed in 3s")
	}
}

func TestTheFirstMessageCarriesEveryRegionAndARunsEventSendsOnlyTheRegionsItTouches(t *testing.T) {
	w := setup(t)
	conn := w.dial(t, "/runs/"+runA+"/pipeline")
	if got := next(t, conn); got != "rail,run-head,pipeline" {
		t.Fatalf("first %q", got)
	}
	w.bus.Publish(sse.KindRuns, sse.HubTarget, map[string]any{})
	if got := next(t, conn); got != "rail,run-head" {
		t.Fatalf("after runs %q", got)
	}
	w.bus.Publish(sse.KindJob, runA, map[string]any{})
	if got := next(t, conn); got != "pipeline" {
		t.Fatalf("after job %q", got)
	}
}

func TestEventsInsideOneFlushWindowLeaveAsOneMessage(t *testing.T) {
	w := setup(t)
	conn := w.dial(t, "/runs/"+runA+"/pipeline")
	next(t, conn)
	for range 5 {
		w.bus.Publish(sse.KindJob, runA, map[string]any{})
	}
	w.bus.Publish(sse.KindRuns, sse.HubTarget, map[string]any{})
	if got := next(t, conn); got != "rail,run-head,pipeline" {
		t.Fatalf("coalesced %q", got)
	}
	quiet(t, conn, 100*time.Millisecond)
}

func TestAnotherRunsJobEventSendsNothingToThisRunsScreen(t *testing.T) {
	w := setup(t)
	conn := w.dial(t, "/runs/"+runA+"/pipeline")
	next(t, conn)
	w.bus.Publish(sse.KindJob, runB, map[string]any{})
	quiet(t, conn, 100*time.Millisecond)
	w.bus.Publish(sse.KindJob, runA, map[string]any{})
	if got := next(t, conn); got != "pipeline" {
		t.Fatalf("canary %q", got)
	}
}

func TestNewSignalsAndARefusedReadMarkTheSignalRegions(t *testing.T) {
	w := setup(t)
	w.signals.add(3)
	conn := w.dial(t, "/runs/"+runA+"/signals/events")
	if got := next(t, conn); got != "rail,run-head,signals" {
		t.Fatalf("first %q", got)
	}
	quiet(t, conn, 60*time.Millisecond)
	w.signals.add(1200)
	if got := next(t, conn); got != "signals" {
		t.Fatalf("after signals %q", got)
	}
	w.signals.refuse()
	if got := next(t, conn); got != "signals" {
		t.Fatalf("after refusal %q", got)
	}
	quiet(t, conn, 60*time.Millisecond)
}

func TestAGoneRunAndAnUnknownScreenCloseNormallySoTheTabDoesNotReconnect(t *testing.T) {
	w := setup(t)
	conn := w.dial(t, "/runs/"+runA+"/pipeline")
	next(t, conn)
	w.setGone()
	w.bus.Publish(sse.KindRuns, sse.HubTarget, map[string]any{})
	closedNormally(t, conn)
	for _, screen := range []string{"/nowhere", "/hub"} {
		closedNormally(t, w.dial(t, screen))
	}
}

func TestAHubScreenSeesRunsEventsButNoRunJobEvents(t *testing.T) {
	w := setup(t)
	conn := w.dial(t, "/")
	if got := next(t, conn); got != "rail,runs" {
		t.Fatalf("first %q", got)
	}
	w.bus.Publish(sse.KindJob, runA, map[string]any{})
	quiet(t, conn, 100*time.Millisecond)
	w.bus.Publish(sse.KindRuns, sse.HubTarget, map[string]any{})
	if got := next(t, conn); got != "rail,runs" {
		t.Fatalf("after runs %q", got)
	}
}

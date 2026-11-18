package recorder

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/wire"
)

func TestARecordedCallCarriesItsRouteMethodQueryCallerAndParsedBodies(t *testing.T) {
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		io.Copy(io.Discard, req.Body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}), nil)
	res, err := r.client.Post(r.url("/items?kind=a"), "application/json", strings.NewReader(`{"n":1}`))
	if err != nil {
		t.Fatal(err)
	}
	drain(res)
	c := r.onlyCall()
	if c.Seq != 1 || c.Caller != "orders" || c.Target != "127.0.0.1" || c.Port != portOf(t, r.upstream) || c.Method != "POST" || c.Path != "/items" || c.Status != 200 || c.Outcome != wire.OutcomeAnswered {
		t.Fatalf("call %+v", c)
	}
	if c.Query.Form["kind"][0] != "a" || c.Request.Body.Kind != wire.BodyJSON || c.Response.Body.Kind != wire.BodyJSON || c.Request.Size != 7 {
		t.Fatalf("call %+v", c)
	}
	if got := c.Request.Headers["host"]; len(got) != 1 || got[0] != r.addr() {
		t.Fatalf("the caller's host header %v", got)
	}
	if c.Finished.Before(c.Started) {
		t.Fatalf("finished %v before started %v", c.Finished, c.Started)
	}
}

type stampSeenWriter struct {
	http.ResponseWriter
	call *call
	seen []time.Time
}

func (w *stampSeenWriter) note() {
	w.call.mu.Lock()
	defer w.call.mu.Unlock()
	w.seen = append(w.seen, w.call.lastWrite)
}

func (w *stampSeenWriter) WriteHeader(int) { w.note() }

func (w *stampSeenWriter) Write(p []byte) (int, error) {
	w.note()
	return len(p), nil
}

func TestTheAnswerIsStampedBeforeItsBytesLeaveTheRecorder(t *testing.T) {
	c := &call{}
	inner := &stampSeenWriter{ResponseWriter: httptest.NewRecorder(), call: c}
	out := &callerWriter{ResponseWriter: inner, call: c}
	beforeHeader := time.Now()
	out.WriteHeader(http.StatusOK)
	beforeBody := time.Now()
	io.WriteString(out, "body")
	if len(inner.seen) != 2 {
		t.Fatalf("%d writes reached the caller", len(inner.seen))
	}
	if inner.seen[0].Before(beforeHeader) || inner.seen[1].Before(beforeBody) {
		t.Fatalf("the writes left with the stamps %v, begun at %v and %v", inner.seen, beforeHeader, beforeBody)
	}
}

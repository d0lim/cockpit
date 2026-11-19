package recorder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/wire"
)

func storeWithLimits(t *testing.T, tune func(*Limits)) *store {
	t.Helper()
	l := testLimits()
	tune(&l)
	names := newCallerNames(testProject, func(context.Context, string) ([]string, error) { return nil, context.Canceled })
	return newStore(l, testMasker(t), names)
}

func addCall(t *testing.T, s *store, request, response string) *call {
	t.Helper()
	r := httptest.NewRequest("POST", "/c", strings.NewReader(request))
	c := s.begin(Route{Service: "orders", Port: 8080}, r)
	if c == nil {
		t.Fatal("the store is not recording")
	}
	c.add(&c.request, []byte(request))
	c.ended(&c.request)
	c.responded(&http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/plain"}}, ContentLength: -1})
	c.add(&c.response, []byte(response))
	c.ended(&c.response)
	c.markServed()
	return c
}

func takeFrom(t *testing.T, s *store) taken {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.work(ctx)
	rec, err := s.takeRecord(ctx, InitialTake)
	if err != nil {
		t.Fatal(err)
	}
	var raw strings.Builder
	for _, l := range rec.lines {
		raw.Write(l)
		raw.WriteByte('\n')
	}
	raw.WriteString(`{"end":{"calls":` + strconv.Itoa(rec.end.Calls) + `,"bodiesOmitted":` + strconv.Itoa(rec.end.BodiesOmitted) + `,"callsDropped":` + strconv.Itoa(rec.end.CallsDropped) + "}}\n")
	return parseTake(t, raw.String())
}

func TestBodiesPastThePendingBudgetAreOmittedAsBusyAndTheirCallStays(t *testing.T) {
	s := storeWithLimits(t, func(l *Limits) { l.Pending = 8 })
	s.finish(addCall(t, s, "12345678", "over"))
	s.finish(addCall(t, s, "12", "34"))
	got := takeFrom(t, s)
	if len(got.calls) != 2 || got.end.BodiesOmitted != 2 {
		t.Fatalf("take %+v", got)
	}
	over := got.calls[0]
	if over.Request.Body.Omitted != wire.OmitBusy || over.Response.Body.Omitted != wire.OmitBusy || over.Request.Size != 8 || over.Response.Size != 4 {
		t.Fatalf("the call past the budget %+v", over)
	}
	if got.calls[1].Request.Body.Text != "12" || got.calls[1].Response.Body.Text != "34" {
		t.Fatalf("the call after the budget freed %+v", got.calls[1])
	}
	if strings.Contains(got.raw, "12345678") {
		t.Fatalf("an omitted body stayed in the record: %s", got.raw)
	}
	if pending := s.pending.Load(); pending != 0 {
		t.Fatalf("%d bytes stay reserved after the take", pending)
	}
}

func TestLaterCallsKeepNoBodiesOnceTheBodyBudgetIsUsed(t *testing.T) {
	s := storeWithLimits(t, func(l *Limits) { l.WithBodies = 1200 })
	for range 4 {
		s.finish(addCall(t, s, strings.Repeat("a", 200), "reply"))
	}
	got := takeFrom(t, s)
	if len(got.calls) != 4 {
		t.Fatalf("%d calls", len(got.calls))
	}
	last := got.calls[3]
	if last.Request.Body.Omitted != wire.OmitRecordFull || last.Response.Body.Omitted != wire.OmitRecordFull || last.Request.Size != 200 {
		t.Fatalf("the last call %+v", last)
	}
	if got.calls[0].Request.Body.Kind != wire.BodyText {
		t.Fatalf("the first call lost its body: %+v", got.calls[0])
	}
	if got.end.BodiesOmitted < 2 {
		t.Fatalf("end %+v", got.end)
	}
}

func TestCallsPastTheLineBudgetAreCountedAsDropped(t *testing.T) {
	s := storeWithLimits(t, func(l *Limits) { l.WithBodies, l.Lines = 300, 700 })
	for range 8 {
		s.finish(addCall(t, s, "body", "reply"))
	}
	got := takeFrom(t, s)
	if len(got.calls) == 0 || len(got.calls) == 8 || got.end.CallsDropped != 8-len(got.calls) || got.end.Calls != len(got.calls) {
		t.Fatalf("%d calls, end %+v", len(got.calls), got.end)
	}
}

func TestAFullQueueCountsTheCallAsDropped(t *testing.T) {
	s := storeWithLimits(t, func(l *Limits) { l.Queue = 1 })
	for range 3 {
		s.finish(addCall(t, s, "body", "reply"))
	}
	got := takeFrom(t, s)
	if len(got.calls) != 1 || got.end.CallsDropped != 2 {
		t.Fatalf("%d calls, end %+v", len(got.calls), got.end)
	}
	if pending := s.pending.Load(); pending != 0 {
		t.Fatalf("%d bytes stay reserved after dropped calls", pending)
	}
}

func TestABodyOverTheLimitPassesWholeAndIsStoredAsTooLarge(t *testing.T) {
	received := make(chan int, 1)
	reply := strings.Repeat("r", wire.MaxBody+5)
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		n := 0
		buf := make([]byte, 64<<10)
		for {
			read, err := req.Body.Read(buf)
			n += read
			if err != nil {
				break
			}
		}
		received <- n
		w.Write([]byte(reply))
	}), nil)
	size := wire.MaxBody + 1
	res, err := r.client.Post(r.url("/big"), "text/plain", strings.NewReader(strings.Repeat("q", size)))
	if err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	buf := make([]byte, 64<<10)
	for {
		n, rerr := res.Body.Read(buf)
		got.Write(buf[:n])
		if rerr != nil {
			break
		}
	}
	res.Body.Close()
	if <-received != size || got.Len() != len(reply) {
		t.Fatalf("upstream received a different size, or the caller read %d of %d", got.Len(), len(reply))
	}
	c := r.onlyCall()
	if c.Request.Body.Omitted != wire.OmitTooLarge || c.Request.Size != int64(size) || c.Response.Body.Omitted != wire.OmitTooLarge || c.Response.Size != int64(len(reply)) {
		t.Fatalf("call request %+v response %+v", c.Request, c.Response)
	}
}

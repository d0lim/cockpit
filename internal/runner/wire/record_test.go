package recorder

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/wire"
)

func TestARecordedCallCarriesItsRouteMethodQueryCallerAndParsedBodies(t *testing.T) {
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

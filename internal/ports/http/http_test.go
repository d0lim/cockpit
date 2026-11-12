package httpport

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/tracectx"
)

const planted = "PLANTED-token-7f3a9c"

var httpMutations = []porttest.Mutation{
	{"requests outside the allowed origins are rejected", "remove the origin check", "red: requests to another host, an IP, or https pass", "TestOnlyAllowedServiceNamesPassAndOtherHostsIPsHTTPSOrRelativePathsArePortDenied"},
	{"rejected requests are not counted", "increment Sent before the check", "red: Sent is not 0 after rejected requests", "TestOnlyAllowedServiceNamesPassAndOtherHostsIPsHTTPSOrRelativePathsArePortDenied"},
	{"a denial says how to allow the origin", "drop the env.json hint from the denial message", "red: the message does not mention env.json", "TestDeniedOriginMessageNamesTheTargetAndHowToAllowIt"},
	{"only bare service origins count as origins", "accept any URL in Origin", "red: an IP, a path or ftp yields an origin", "TestOriginAcceptsSchemeHostAndPortOnly"},
	{"redirects are rejected", "remove the 3xx check", "red: a 302 response comes back as a Response", "TestRedirectResponseIsPortDeniedAndCountsAsSent"},
	{"time limit", "remove WithTimeout", "red: the test hangs on a slow server", "TestExceedingTheTimeLimitIsPortTimeoutAndResponsesWithinItPassThrough"},
	{"body size limit", "remove LimitReader", "red: a body over the limit comes back as is", "TestBodyOverTheLimitIsPortLimitAndBodyWithinItIsNotTruncated"},
	{"error messages are masked", "build the rejection message without mask", "red: the value planted in the path shows in the error message", "TestDenialAndFailureMessagesPassTheMaskerAndOmitTheQuery"},
	{"proxy environment variables are ignored", "set Transport.Proxy to ProxyFromEnvironment", "red: with HTTP_PROXY set, requests go to the proxy", "TestOnlyAllowedServiceNamesPassAndOtherHostsIPsHTTPSOrRelativePathsArePortDenied"},
	{"every call carries the run trace id and records the span id it sent", "record a fresh span id instead of the one put in the header", "red: the recorded span id differs from the one the server received", "TestEveryCallSendsATraceparentWithTheRunTraceID"},
	{"a script traceparent goes out unchanged", "set the runner traceparent without looking at the script headers", "red: the server receives the run trace id instead of the script value", "TestAScriptTraceparentWinsAndItsSpanIDIsRecorded"},
	{"a padded script traceparent records the span id of the trimmed value sent", "parse the script value without TrimString", "red: the padded value records no span id", "TestAScriptTraceparentWinsAndItsSpanIDIsRecorded"},
	{"a fresh span id is one the masker leaves unchanged", "return the first random span id without asking the masker", "red: a span id with a masked character goes out", "TestAFreshSpanIDIsNeverOneTheMaskerWouldChange"},
	{"no trace id means no traceparent", "remove the empty trace id check", "red: a traceparent without a trace id reaches the server", "TestNoTraceIDMeansNoTraceparent"},
}

func server(t *testing.T) (*Client, *httptest.Server) {
	t.Helper()
	return tracedServer(t, "", plantedMask)
}

func plantedMask(s string) string { return strings.ReplaceAll(s, planted, "****") }

func tracedServer(t *testing.T, traceID string, mask ports.Mask) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/echo":
			body, _ := io.ReadAll(r.Body)
			w.Header().Set("X-Echo-Content-Type", r.Header.Get("Content-Type"))
			w.Header().Set("X-Echo-Auth", r.Header.Get("Authorization"))
			w.Header()["X-Echo-Traceparent"] = r.Header.Values("Traceparent")
			w.Header().Add("X-Multi", "a")
			w.Header().Add("X-Multi", "b")
			w.WriteHeader(201)
			w.Write(body)
		case "/redirect":
			w.Header().Set("Location", "/echo")
			w.WriteHeader(302)
		case "/slow":
			time.Sleep(300 * time.Millisecond)
			w.WriteHeader(200)
		case "/big":
			w.Write([]byte(strings.Repeat("x", 1025)))
		case "/exact":
			w.Write([]byte(strings.Repeat("y", 1024)))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	c, err := New(Options{
		Allowed: []string{"http://app:8080", "http://mock:9090"},
		Timeout: 100 * time.Millisecond,
		MaxBody: 1024,
		Mask:    mask,
		Dial: func(ctx context.Context, network, dialed string) (net.Conn, error) {
			if dialed != "app:8080" && dialed != "mock:9090" {
				return nil, errors.New("unexpected dial " + dialed)
			}
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func codeOf(err error) engine.PortCode {
	var pe *engine.PortError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func TestOnlyAllowedServiceNamesPassAndOtherHostsIPsHTTPSOrRelativePathsArePortDenied(t *testing.T) {
	c, _ := server(t)
	res, err := c.Send(context.Background(), Request{Method: "POST", URL: "http://app:8080/echo?q=1", Headers: map[string]string{"Authorization": "Bearer x"}, JSON: map[string]any{"a": 1}})
	if err != nil || res.Status != 201 || res.Body != `{"a":1}` || res.Headers["x-echo-content-type"] != "application/json" || res.Headers["x-echo-auth"] != "Bearer x" || res.Headers["x-multi"] != "a, b" || res.Method != "POST" || res.URL != "http://app:8080/echo?q=1" {
		t.Fatalf("allowed request: %+v %v", res, err)
	}
	if n := c.Counter(); n.Sent != 1 || n.LastStatus != 201 {
		t.Fatalf("counter: %+v", n)
	}
	c.ResetCounter()
	denied := []string{"http://evil:8080/echo", "http://127.0.0.1:1/echo", "https://app:8080/echo", "/echo", "http://app:9999/echo", "http://app/echo"}
	for _, u := range denied {
		if _, err := c.Send(context.Background(), Request{Method: "GET", URL: u}); codeOf(err) != engine.PortDenied {
			t.Fatalf("%s: %v", u, err)
		}
	}
	if n := c.Counter(); n.Sent != 0 || n.LastStatus != 0 {
		t.Fatalf("counter after denials: %+v (%d denied)", n, len(denied))
	}
	if _, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://mock:9090/echo"}); err != nil {
		t.Fatalf("second allowed origin: %v", err)
	}
}

func TestDeniedOriginMessageNamesTheTargetAndHowToAllowIt(t *testing.T) {
	c, _ := server(t)
	_, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://evil:8080/echo?token=" + planted})
	if codeOf(err) != engine.PortDenied || !strings.Contains(err.Error(), "GET /echo") || !strings.Contains(err.Error(), "as a value in env.json") || strings.Contains(err.Error(), planted) {
		t.Fatalf("%v", err)
	}
}

func TestOriginAcceptsSchemeHostAndPortOnly(t *testing.T) {
	for in, want := range map[string]string{"http://web": "http://web", "http://web:8080/": "http://web:8080", "https://api.local": "https://api.local"} {
		if got, ok := Origin(in); !ok || got != want {
			t.Fatalf("%s: %q %v", in, got, ok)
		}
	}
	for _, in := range []string{"http://web/api", "http://127.0.0.1:8080", "ftp://web", "web:8080", "", "http://"} {
		if got, ok := Origin(in); ok {
			t.Fatalf("%s accepted as %q", in, got)
		}
	}
}

func TestRedirectResponseIsPortDeniedAndCountsAsSent(t *testing.T) {
	c, _ := server(t)
	_, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://app:8080/redirect"})
	if codeOf(err) != engine.PortDenied || !strings.Contains(err.Error(), "redirect response") {
		t.Fatalf("%v", err)
	}
	if n := c.Counter(); n.Sent != 1 || n.LastStatus != 302 {
		t.Fatalf("%+v", n)
	}
}

func TestExceedingTheTimeLimitIsPortTimeoutAndResponsesWithinItPassThrough(t *testing.T) {
	c, _ := server(t)
	_, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://app:8080/slow"})
	if codeOf(err) != engine.PortTimeout || !strings.Contains(err.Error(), "100ms") || !strings.Contains(err.Error(), "GET /slow") {
		t.Fatalf("%v", err)
	}
	res, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://app:8080/slow", TimeoutMs: 2000})
	if err != nil || res.Status != 200 {
		t.Fatalf("per-request limit: %+v %v", res, err)
	}
	if n := c.Counter(); n.Sent != 2 || n.LastStatus != 200 {
		t.Fatalf("%+v", n)
	}
	c.ResetStatus()
	if n := c.Counter(); n.Sent != 2 || n.LastStatus != 0 {
		t.Fatalf("only the status is reset: %+v", n)
	}
}

func TestBodyOverTheLimitIsPortLimitAndBodyWithinItIsNotTruncated(t *testing.T) {
	c, _ := server(t)
	if _, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://app:8080/big"}); codeOf(err) != engine.PortLimit {
		t.Fatalf("%v", err)
	}
	res, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://app:8080/exact"})
	if err != nil || len(res.Body) != 1024 {
		t.Fatalf("%d %v", len(res.Body), err)
	}
}

func TestDenialAndFailureMessagesPassTheMaskerAndOmitTheQuery(t *testing.T) {
	c, srv := server(t)
	_, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://evil:8080/" + planted + "?token=" + planted})
	if err == nil || strings.Contains(err.Error(), planted) || !strings.Contains(err.Error(), "****") || strings.Contains(err.Error(), "token=") {
		t.Fatalf("%v", err)
	}
	srv.Close()
	_, err = c.Send(context.Background(), Request{Method: "GET", URL: "http://app:8080/" + planted + "?token=" + planted})
	if codeOf(err) != engine.PortUnavailable || strings.Contains(err.Error(), planted) || strings.Contains(err.Error(), "token=") || !strings.Contains(err.Error(), "GET /****") {
		t.Fatalf("%v", err)
	}
	if _, err := New(Options{Allowed: []string{"app:8080"}, Mask: ports.Identity}); err == nil {
		t.Fatal("allowed origin without a scheme")
	}
}

func echoedTraceparent(t *testing.T, c *Client, headers map[string]string) (string, Call) {
	t.Helper()
	res, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://app:8080/echo", Headers: headers})
	calls := c.TakeCalls()
	if err != nil || res.Status != 201 || len(calls) != 1 {
		t.Fatalf("echo: %+v %v %+v", res, err, calls)
	}
	return res.Headers["x-echo-traceparent"], calls[0]
}

func TestEveryCallSendsATraceparentWithTheRunTraceID(t *testing.T) {
	runTrace := tracectx.TraceID("run-20261012-153001-997a37e")
	c, _ := tracedServer(t, runTrace, plantedMask)
	seen := map[string]bool{}
	for range 3 {
		header, call := echoedTraceparent(t, c, map[string]string{"Authorization": "Bearer x"})
		trace, span, ok := tracectx.Parse(header)
		if !ok || trace != runTrace || span != call.SpanID || header != tracectx.Format(runTrace, span) || seen[span] {
			t.Fatalf("traceparent %q, recorded span id %q, earlier %v", header, call.SpanID, seen)
		}
		seen[span] = true
	}
	if len(seen) != 3 {
		t.Fatalf("%d distinct span ids", len(seen))
	}
}

func TestAScriptTraceparentWinsAndItsSpanIDIsRecorded(t *testing.T) {
	c, _ := tracedServer(t, tracectx.TraceID("run-20261012-153001-997a37e"), plantedMask)
	script := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	if header, call := echoedTraceparent(t, c, map[string]string{"traceparent": script}); header != script || call.SpanID != "00f067aa0ba902b7" {
		t.Fatalf("script traceparent: sent %q, recorded %q", header, call.SpanID)
	}
	if header, call := echoedTraceparent(t, c, map[string]string{"traceparent": " " + script + "\t"}); header != script || call.SpanID != "00f067aa0ba902b7" {
		t.Fatalf("padded script traceparent: sent %q, recorded %q", header, call.SpanID)
	}
	if header, call := echoedTraceparent(t, c, map[string]string{"Traceparent": "not-a-traceparent"}); header != "not-a-traceparent" || call.SpanID != "" {
		t.Fatalf("malformed script traceparent: sent %q, recorded %q", header, call.SpanID)
	}
	if header, call := echoedTraceparent(t, c, nil); call.SpanID == "" || header != tracectx.Format(tracectx.TraceID("run-20261012-153001-997a37e"), call.SpanID) {
		t.Fatalf("a later call without a script value: sent %q, recorded %q", header, call.SpanID)
	}
}

func TestAFreshSpanIDIsNeverOneTheMaskerWouldChange(t *testing.T) {
	runTrace := tracectx.TraceID("run-20261012-153001-997a37e")
	c, _ := tracedServer(t, runTrace, func(s string) string { return strings.ReplaceAll(s, "0", "*") })
	for range 20 {
		header, call := echoedTraceparent(t, c, nil)
		if _, span, ok := tracectx.Parse(header); !ok || span != call.SpanID || strings.Contains(span, "0") {
			t.Fatalf("sent %q, recorded %q", header, call.SpanID)
		}
	}
}

func TestNoTraceIDMeansNoTraceparent(t *testing.T) {
	c, _ := server(t)
	res, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://app:8080/echo", Headers: map[string]string{"Authorization": "Bearer x"}})
	calls := c.TakeCalls()
	if err != nil || res.Headers["x-echo-auth"] != "Bearer x" || len(calls) != 1 {
		t.Fatalf("echo: %+v %v %+v", res, err, calls)
	}
	if header, has := res.Headers["x-echo-traceparent"]; has || calls[0].SpanID != "" {
		t.Fatalf("a client without a trace id sent %q and recorded %q", header, calls[0].SpanID)
	}
}

func TestHTTPMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "http_test.go", httpMutations)
}

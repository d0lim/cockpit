package recorder

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/wire"
)

type seenRequest struct {
	method, uri, host string
	headers           http.Header
	body              []byte
}

func recordingUpstream(seen chan<- seenRequest, reply http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen <- seenRequest{r.Method, r.RequestURI, r.Host, r.Header.Clone(), body}
		reply(w, r)
	}
}

func exchangeThrough(t *testing.T, client *http.Client, url string, payload []byte) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "orders.example:8080"
	req.Header["X-Forwarded-For"] = []string{"203.0.113.9"}
	req.Header["Forwarded"] = []string{"for=203.0.113.9"}
	req.Header["X-Custom"] = []string{"one", "two"}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res, body
}

func TestARequestAndItsResponsePassThroughUnchanged(t *testing.T) {
	payload := []byte{0, 1, 2, 0xff, 'x', '\n'}
	reply := []byte{9, 8, 0, 0xfe}
	seen := make(chan seenRequest, 2)
	r := startRig(t, recordingUpstream(seen, func(w http.ResponseWriter, _ *http.Request) {
		h := w.Header()
		h["X-Reply"] = []string{"a", "b"}
		h["Content-Type"] = nil
		h["Date"] = nil
		w.WriteHeader(http.StatusTeapot)
		w.Write(reply)
	}), nil)
	path := "/a%2Fb/c?x=1;y=2&bad=%zz"
	direct, directBody := exchangeThrough(t, r.client, "http://"+r.upstream+path, payload)
	directSeen := <-seen
	proxied, proxiedBody := exchangeThrough(t, r.client, r.url(path), payload)
	proxiedSeen := <-seen
	if proxied.StatusCode != http.StatusTeapot || direct.StatusCode != proxied.StatusCode {
		t.Fatalf("status direct %d proxied %d", direct.StatusCode, proxied.StatusCode)
	}
	if !bytes.Equal(proxiedBody, reply) || !bytes.Equal(directBody, reply) {
		t.Fatalf("body direct %q proxied %q", directBody, proxiedBody)
	}
	if !maps.EqualFunc(direct.Header, proxied.Header, slices.Equal[[]string]) {
		t.Fatalf("response headers direct %v proxied %v", direct.Header, proxied.Header)
	}
	if proxiedSeen.method != "POST" || proxiedSeen.uri != directSeen.uri || !bytes.Equal(proxiedSeen.body, payload) {
		t.Fatalf("upstream saw %+v, direct %+v", proxiedSeen, directSeen)
	}
	proxiedSeen.headers.Del("Connection")
	if !maps.EqualFunc(directSeen.headers, proxiedSeen.headers, slices.Equal[[]string]) {
		t.Fatalf("request headers direct %v proxied %v", directSeen.headers, proxiedSeen.headers)
	}
	if directSeen.host != "orders.example:8080" || proxiedSeen.host != r.upstream {
		t.Fatalf("host direct %q proxied %q, want the upstream %q", directSeen.host, proxiedSeen.host, r.upstream)
	}
	if len(proxiedSeen.headers["X-Custom"]) != 2 || proxiedSeen.headers.Get("X-Forwarded-For") != "203.0.113.9" {
		t.Fatalf("the canary headers are missing: %v", proxiedSeen.headers)
	}
}

func TestTheProxyAddsNoForwardingHeadersOfItsOwn(t *testing.T) {
	seen := make(chan seenRequest, 1)
	r := startRig(t, recordingUpstream(seen, func(http.ResponseWriter, *http.Request) {}), nil)
	r.get("/plain")
	got := <-seen
	for _, name := range forwardingHeaders {
		if _, ok := got.headers[name]; ok {
			t.Errorf("the proxy added %s: %v", name, got.headers[name])
		}
	}
	if got.uri != "/plain" || got.host != r.upstream {
		t.Fatalf("the request did not reach the upstream as sent: %+v", got)
	}
}

func TestAStreamingResponseReachesTheCallerBeforeTheUpstreamFinishes(t *testing.T) {
	release := make(chan struct{})
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "first\n")
		w.(http.Flusher).Flush()
		<-release
		io.WriteString(w, "second\n")
	}), nil)
	defer close(release)
	res, err := r.client.Get(r.url("/stream"))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	in := bufio.NewReader(res.Body)
	got := make(chan string, 1)
	go func() {
		text, _ := in.ReadString('\n')
		got <- text
	}()
	select {
	case text := <-got:
		if text != "first\n" {
			t.Fatalf("first chunk %q", text)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the first chunk waited for the upstream to finish")
	}
}

func TestResponseHeadersReachTheCallerWhileTheUpstreamStaysSilent(t *testing.T) {
	release := make(chan struct{})
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		w.(http.Flusher).Flush()
		<-release
	}), nil)
	defer close(release)
	done := make(chan int, 1)
	go func() {
		res, err := r.client.Get(r.url("/silent"))
		if err != nil {
			done <- 0
			return
		}
		res.Body.Close()
		done <- res.StatusCode
	}()
	select {
	case status := <-done:
		if status != http.StatusAccepted {
			t.Fatalf("status %d", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the headers waited for the first body byte")
	}
}

func closedPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func TestADialFailureClosesTheCallerConnectionAndRecordsNoUpstream(t *testing.T) {
	r := startRigAt(t, closedPort(t), nil)
	conn, err := net.Dial("tcp", r.addr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET /down?x=1 HTTP/1.1\r\nHost: orders:8080\r\n\r\n")
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	answer, err := io.ReadAll(conn)
	if err != nil || len(answer) != 0 {
		t.Fatalf("the caller read %q, err %v, want a closed connection with no invented response", answer, err)
	}
	c := r.onlyCall()
	if c.Outcome != wire.OutcomeNoUpstream || c.Status != 0 || c.Path != "/down" || c.Finished.IsZero() {
		t.Fatalf("call %+v", c)
	}
}

func TestAResponseThatBreaksOffStillRecordsItsStatusAndSizeWithoutABody(t *testing.T) {
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			panic(err)
		}
		defer conn.Close()
		io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 100\r\nContent-Type: text/plain\r\n\r\npartial")
	}), nil)
	res, err := r.client.Get(r.url("/cut"))
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(res.Body)
	res.Body.Close()
	if readErr == nil || string(body) != "partial" {
		t.Fatalf("the caller read %q, err %v, want the partial body and an error", body, readErr)
	}
	c := r.onlyCall()
	if c.Outcome != wire.OutcomeIncomplete || c.Status != 200 || c.Response.Size != 7 || c.Response.Body.Kind != "" {
		t.Fatalf("call %+v", c)
	}
}

func TestARouteBoundToLoopbackCannotBeShadowedByAnotherListener(t *testing.T) {
	r := startRig(t, okUpstream(), nil)
	if host, _, err := net.SplitHostPort(r.rec.RouteAddr(0)); err != nil || host != "127.0.0.1" {
		t.Fatalf("the route listens on %s (%v)", r.rec.RouteAddr(0), err)
	}
	if shadow, err := net.Listen("tcp", r.addr()); err == nil {
		shadow.Close()
		t.Fatalf("a second listener took %s", r.addr())
	}
	r.get("/x")
	if c := r.onlyCall(); c.Path != "/x" {
		t.Fatalf("call %+v", c)
	}
}

func TestARouteWithoutABindHostListensOnEveryInterface(t *testing.T) {
	r := startRig(t, okUpstream(), func(o *Options) { o.BindHost = "" })
	host, _, err := net.SplitHostPort(r.rec.RouteAddr(0))
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsUnspecified() {
		t.Fatalf("the route listens on %s (%v)", r.rec.RouteAddr(0), err)
	}
}

func TestACallerThatLeavesIsRecordedAsCallerGone(t *testing.T) {
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, "first\n")
		w.(http.Flusher).Flush()
		<-req.Context().Done()
	}), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", r.url("/leave"), nil)
	res, err := r.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(res.Body).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	cancel()
	res.Body.Close()
	if c := r.onlyCall(); c.Outcome != wire.OutcomeCallerGone || c.Status != 200 {
		t.Fatalf("call %+v", c)
	}
}

func TestAnUpgradeIsTunneledAndRecordedWithItsStatus(t *testing.T) {
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			panic(err)
		}
		defer conn.Close()
		io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: echo\r\n\r\n")
		text, _ := buf.ReadString('\n')
		io.WriteString(conn, strings.ToUpper(text))
	}), nil)
	conn, err := net.Dial("tcp", r.addr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprintf(conn, "GET /tunnel HTTP/1.1\r\nHost: orders:8080\r\nConnection: Upgrade\r\nUpgrade: echo\r\n\r\n")
	in := bufio.NewReader(conn)
	res, err := http.ReadResponse(in, nil)
	if err != nil || res.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("response %+v err %v", res, err)
	}
	io.WriteString(conn, "hello\n")
	if text, err := in.ReadString('\n'); err != nil || text != "HELLO\n" {
		t.Fatalf("tunnel answered %q err %v", text, err)
	}
	conn.Close()
	if c := r.onlyCall(); c.Status != http.StatusSwitchingProtocols || c.Outcome != wire.OutcomeAnswered {
		t.Fatalf("call %+v", c)
	}
}

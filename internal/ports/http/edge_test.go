package httpport

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var edgeMutations = []porttest.Mutation{
	{"only GET and HEAD reach the edge", "drop the method check in resolve", "red: the POST reaches the server", "TestGetAndHeadPassAndPostIsDeniedWithoutReachingTheEdge"},
	{"redirects are refused and not followed", "follow redirects in CheckRedirect", "red: the redirect target receives a request", "TestRedirectIsReportedAsDeniedAndItsTargetIsNeverRequested"},
	{"only edge names of this run resolve", "dial the URL host when it is not an edge", "red: localhost and the hub port are reached", "TestHubAPIArbitraryLocalhostPortsIPsAndForeignHostsAreDenied"},
	{"Dialer.Control allows only the run's edge ports", "Control returns nil", "red: a dial to another loopback port passes Control", "TestDialerControlAllowsOnlyTheRunEdgeAddresses"},
	{"proxy environment variables are ignored", "set Transport.Proxy to ProxyFromEnvironment", "red: the proxy receives the request", "TestProxyEnvironmentIsIgnored"},
	{"errors pass the masker", "build the denial without mask", "red: the planted path shows in the error", "TestDenialMessagesPassTheMaskerAndOmitTheQuery"},
	{"the body limit holds and bodies within it are not truncated", "remove LimitReader", "red: an oversized body comes back", "TestBodyOverTheLimitIsPortLimitAndBodyWithinItIsWhole"},
	{"script arguments decode into the request", "ignore the options argument", "red: the header does not reach the edge", "TestEdgeCallDecodesSendArgumentsLikeTheScriptPassesThem"},
}

func edgeServer(t *testing.T, handler http.HandlerFunc) (*EdgeClient, *atomic.Int32, int) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	c, err := NewEdge(EdgeOptions{Ports: map[string]int{"app": port}, MaxBody: 1024})
	if err != nil {
		t.Fatal(err)
	}
	return c, &hits, port
}

func code(err error) engine.PortCode {
	var pe *engine.PortError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func TestGetAndHeadPassAndPostIsDeniedWithoutReachingTheEdge(t *testing.T) {
	c, hits, _ := edgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("health " + r.Method + " " + r.URL.RequestURI()))
	})
	res, err := c.Send(context.Background(), EdgeRequest{Method: "GET", URL: "http://app/health?deep=1"})
	if err != nil || res.Status != 200 || res.Body != "health GET /health?deep=1" || res.URL != "http://app/health?deep=1" {
		t.Fatalf("%+v %v", res, err)
	}
	if res, err := c.Send(context.Background(), EdgeRequest{Method: "HEAD", URL: "http://app/health"}); err != nil || res.Status != 200 {
		t.Fatalf("head: %+v %v", res, err)
	}
	before := hits.Load()
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		_, err := c.Send(context.Background(), EdgeRequest{Method: m, URL: "http://app/health"})
		if code(err) != engine.PortDenied || !strings.Contains(err.Error(), "only GET and HEAD") {
			t.Fatalf("%s: %v", m, err)
		}
	}
	if hits.Load() != before {
		t.Fatalf("denied methods reached the edge: %d", hits.Load()-before)
	}
}

func TestRedirectIsReportedAsDeniedAndItsTargetIsNeverRequested(t *testing.T) {
	var targetHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetHits.Add(1) }))
	defer other.Close()
	c, _, _ := edgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.Write([]byte("ok"))
			return
		}
		if r.URL.Path == "/same" {
			w.Header().Set("Location", "/health")
		} else {
			w.Header().Set("Location", other.URL+"/steal")
		}
		w.WriteHeader(302)
	})
	for _, path := range []string{"/away", "/same"} {
		_, err := c.Send(context.Background(), EdgeRequest{Method: "GET", URL: "http://app" + path})
		if code(err) != engine.PortDenied || !strings.Contains(err.Error(), "redirect response (302) that is not followed") {
			t.Fatalf("%s: %v", path, err)
		}
	}
	if targetHits.Load() != 0 {
		t.Fatalf("redirect target requested %d times", targetHits.Load())
	}
	if res, err := c.Send(context.Background(), EdgeRequest{Method: "GET", URL: "http://app/ok"}); err != nil || res.Status != 200 {
		t.Fatalf("canary: %+v %v", res, err)
	}
}

func TestHubAPIArbitraryLocalhostPortsIPsAndForeignHostsAreDenied(t *testing.T) {
	c, hits, port := edgeServer(t, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	urls := []string{
		"http://127.0.0.1:7710/api/runs",
		"http://localhost:7710/api/runs",
		"http://app:7710/api/runs",
		"http://127.0.0.1:" + strconv.Itoa(port) + "/health",
		"http://localhost:" + strconv.Itoa(port) + "/health",
		"http://[::1]:" + strconv.Itoa(port) + "/health",
		"https://app/health",
		"http://user:pw@app/health",
		"http://example.com/health",
		"/health",
	}
	for _, u := range urls {
		if _, err := c.Send(context.Background(), EdgeRequest{Method: "GET", URL: u}); code(err) != engine.PortDenied {
			t.Errorf("%s: %v", u, err)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("denied URLs reached the edge %d times", hits.Load())
	}
	if _, err := c.Send(context.Background(), EdgeRequest{Method: "GET", URL: "http://APP/health"}); err != nil || hits.Load() != 1 {
		t.Fatalf("canary: %v", err)
	}
}

func TestDialerControlAllowsOnlyTheRunEdgeAddresses(t *testing.T) {
	c, err := NewEdge(EdgeOptions{Ports: map[string]int{"app": 22001, "api": 22002}})
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range []string{"127.0.0.1:22001", "127.0.0.1:22002"} {
		if err := c.control("tcp4", addr, nil); err != nil {
			t.Errorf("%s refused: %v", addr, err)
		}
	}
	for _, addr := range []string{"127.0.0.1:7710", "127.0.0.1:22003", "10.0.0.1:22001", "[::1]:22001", "0.0.0.0:22001"} {
		if err := c.control("tcp", addr, nil); err == nil {
			t.Errorf("%s allowed", addr)
		}
	}
	if _, err := NewEdge(EdgeOptions{Ports: map[string]int{"../x": 1}}); err == nil {
		t.Fatal("an edge name that is not path safe was accepted")
	}
}

func TestProxyEnvironmentIsIgnored(t *testing.T) {
	var proxied atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { proxied.Add(1) }))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("NO_PROXY", "")
	c, hits, _ := edgeServer(t, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	if _, err := c.Send(context.Background(), EdgeRequest{Method: "GET", URL: "http://app/health"}); err != nil {
		t.Fatal(err)
	}
	if proxied.Load() != 0 || hits.Load() != 1 {
		t.Fatalf("proxy %d edge %d", proxied.Load(), hits.Load())
	}
}

func TestDenialMessagesPassTheMaskerAndOmitTheQuery(t *testing.T) {
	const planted = "PLANTED-token-7f3a9c"
	c, err := NewEdge(EdgeOptions{Ports: map[string]int{"app": 22001}, Mask: func(s string) string { return strings.ReplaceAll(s, planted, "****") }})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Send(context.Background(), EdgeRequest{Method: "POST", URL: "http://app/" + planted + "?key=" + planted})
	if err == nil || strings.Contains(err.Error(), planted) || !strings.Contains(err.Error(), "POST /****") || strings.Contains(err.Error(), "key=") {
		t.Fatalf("%v", err)
	}
}

func TestBodyOverTheLimitIsPortLimitAndBodyWithinItIsWhole(t *testing.T) {
	c, _, _ := edgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/big" {
			w.Write([]byte(strings.Repeat("x", 1025)))
			return
		}
		w.Write([]byte(strings.Repeat("y", 1024)))
	})
	if _, err := c.Send(context.Background(), EdgeRequest{Method: "GET", URL: "http://app/big"}); code(err) != engine.PortLimit {
		t.Fatalf("%v", err)
	}
	res, err := c.Send(context.Background(), EdgeRequest{Method: "GET", URL: "http://app/exact"})
	if err != nil || len(res.Body) != 1024 {
		t.Fatalf("%d %v", len(res.Body), err)
	}
}

func TestEdgeCallDecodesSendArgumentsLikeTheScriptPassesThem(t *testing.T) {
	c, _, _ := edgeServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(r.Header.Get("X-Trace"))) })
	args := []json.RawMessage{json.RawMessage(`"GET"`), json.RawMessage(`"http://app/health"`), json.RawMessage(`{"headers":{"X-Trace":"t1"},"timeoutMs":500}`)}
	got, err := c.Call(context.Background(), "send", args)
	if err != nil || got.(Response).Body != "t1" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := c.Call(context.Background(), "post", args); code(err) != engine.PortBadArgs {
		t.Fatalf("%v", err)
	}
}

func TestEdgeMutationRecordMatchesTestsInEdgeFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "edge_test.go", edgeMutations)
}

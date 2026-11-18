package httpport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var rerouteMutations = []porttest.Mutation{
	{"a plain origin is rerouted and its URL and Host stay", "ignore Reroute in New", "red: the dial goes to the original address", "TestAPlainRequestDialsTheReroutedAddressAndKeepsItsHost"},
	{"https never takes the plain dialer", "drop the https transport", "red: the https dial goes to the rerouted address", "TestAnHTTPSRequestDialsItsOwnAddressThoughARerouteExists"},
	{"the allowlist runs before any dial", "skip the origin check in Send", "red: the refused origin is dialed", "TestARefusedOriginIsNotDialedThoughARerouteNamesIt"},
}

func TestRerouteMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "reroute_test.go", rerouteMutations)
}

type dialLog struct {
	mu    sync.Mutex
	addrs []string
}

func (d *dialLog) dial(target string) Dialer {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		d.mu.Lock()
		d.addrs = append(d.addrs, addr)
		d.mu.Unlock()
		return (&net.Dialer{}).DialContext(ctx, network, target)
	}
}

func (d *dialLog) seen() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string{}, d.addrs...)
}

func rerouteToProxy(addr string) string {
	if strings.EqualFold(addr, "orders:8080") || strings.EqualFold(addr, "orders:8443") {
		return "cockpit-wire:41001"
	}
	return addr
}

func TestAPlainRequestDialsTheReroutedAddressAndKeepsItsHost(t *testing.T) {
	hosts := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { hosts <- r.Host }))
	defer srv.Close()
	log := &dialLog{}
	c, err := New(Options{Allowed: []string{"http://orders:8080"}, Dial: log.dial(srv.Listener.Addr().String()), Reroute: rerouteToProxy})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Send(context.Background(), Request{Method: "GET", URL: "http://orders:8080/x"})
	if err != nil || res.Status != 200 || res.URL != "http://orders:8080/x" {
		t.Fatalf("response %+v err %v", res, err)
	}
	if got := log.seen(); len(got) != 1 || got[0] != "cockpit-wire:41001" {
		t.Fatalf("dials %v", got)
	}
	if host := <-hosts; host != "orders:8080" {
		t.Fatalf("host %q", host)
	}
	if calls := c.TakeCalls(); len(calls) != 1 || calls[0].Host != "orders" {
		t.Fatalf("calls %+v", calls)
	}
}

func TestAnHTTPSRequestDialsItsOwnAddressThoughARerouteExists(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	log := &dialLog{}
	c, err := New(Options{Allowed: []string{"https://orders:8443"}, Dial: log.dial(srv.Listener.Addr().String()), Reroute: rerouteToProxy})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Send(context.Background(), Request{Method: "GET", URL: "https://orders:8443/x"})
	var pe *engine.PortError
	if !errors.As(err, &pe) {
		t.Fatalf("the untrusted test certificate should fail the request with a port error: %v", err)
	}
	if got := log.seen(); len(got) != 1 || got[0] != "orders:8443" {
		t.Fatalf("dials %v", got)
	}
}

func TestARefusedOriginIsNotDialedThoughARerouteNamesIt(t *testing.T) {
	log := &dialLog{}
	c, err := New(Options{Allowed: []string{"http://listed:80"}, Dial: log.dial("127.0.0.1:1"), Reroute: rerouteToProxy})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Send(context.Background(), Request{Method: "GET", URL: "http://orders:8080/x"})
	if codeOf(err) != engine.PortDenied || len(log.seen()) != 0 {
		t.Fatalf("err %v dials %v", err, log.seen())
	}
}

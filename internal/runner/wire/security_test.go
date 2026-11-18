package recorder

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/wire"
)

var recorderMutations = []porttest.Mutation{
	{"the control listener binds only the loopback address", "listen on all interfaces in Listen", "red: the control port answers on a non-loopback address", "TestTheControlListenerAnswersOnLoopbackAndNotOnAnotherAddress"},
	{"stored lines hold only what wire.Capture cleaned", "write the raw query into the stored call after Capture", "red: the planted value is in the taken line", "TestAStoredLineHoldsNoPlantedSecretFromHeadersQueryOrBodies"},
	{"the capture masker keeps its key rules and declared public fields", "give the store Masker.ValueScan, which has neither", "red: the planted value under the key is in the line", "TestADeclaredPublicFieldStaysReadableAndAnotherValueUnderItsKeyIsMasked"},
	{"a body over the limit leaves no fragment", "keep the buffered first part of a body past the limit and store it as whole", "red: the stored call holds the start of the body", "TestABodyOverTheLimitLeavesNoFragmentInTheRecord"},
	{"a body that broke off leaves no fragment", "store the body of a side that did not end", "red: the partial response is stored", "TestABodyThatBrokeOffLeavesNoFragmentInTheRecord"},
	{"diagnostics carry no received value", "log the first stored line when a take is written", "red: the planted value is in the log", "TestDiagnosticsNameNoReceivedValue"},
}

func TestRecorderMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "security_test.go", recorderMutations)
}

func nonLoopbackIP(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil {
			return n.IP.String()
		}
	}
	t.Fatal("this host has no non-loopback IPv4 address, so the deny side cannot be evaluated")
	return ""
}

func TestTheControlListenerAnswersOnLoopbackAndNotOnAnotherAddress(t *testing.T) {
	r := startRig(t, okUpstream(), nil)
	if _, err := r.control("ping"); err != nil {
		t.Fatalf("ping on loopback: %v", err)
	}
	ip := nonLoopbackIP(t)
	route, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(portOf(t, r.rec.RouteAddr(0)))), 2*time.Second)
	if err != nil {
		t.Fatalf("the route port, which listens on every address, is not reachable on %s, so the deny side proves nothing: %v", ip, err)
	}
	route.Close()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(portOf(t, r.rec.ControlAddr()))), 2*time.Second)
	if err == nil {
		conn.Close()
		t.Fatalf("the control port answered on %s", ip)
	}
}

func TestAStoredLineHoldsNoPlantedSecretFromHeadersQueryOrBodies(t *testing.T) {
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		io.Copy(io.Discard, req.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "sid="+planted)
		io.WriteString(w, `{"sessionToken":"`+planted+`","shown":"reply-visible"}`)
	}), nil)
	req, err := http.NewRequest("POST", r.url("/login?token="+planted+"&page=query-visible"), strings.NewReader(`{"password":"`+planted+`","note":"body-visible"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+planted)
	req.Header.Set("X-Internal", planted)
	req.Header.Set("X-Request-Id", "id-visible")
	res, err := r.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	drain(res)
	raw := r.take(InitialTake).raw
	if strings.Contains(raw, planted) || !strings.Contains(raw, "****") {
		t.Fatalf("masking failed: %s", raw)
	}
	for _, canary := range []string{"reply-visible", "query-visible", "body-visible", "id-visible"} {
		if !strings.Contains(raw, canary) {
			t.Errorf("%s is missing, so the line was not what the test thinks: %s", canary, raw)
		}
	}
}

func TestADeclaredPublicFieldStaysReadableAndAnotherValueUnderItsKeyIsMasked(t *testing.T) {
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"token":"tok-1f2e3d4c","other":{"token":"`+planted+`"}}`)
	}), func(o *Options) { o.Masker = testMasker(t, mask.PublicField{Key: "token", Value: "tok-[0-9a-f]{8}"}) })
	r.get("/x")
	raw := r.take(InitialTake).raw
	if !strings.Contains(raw, "tok-1f2e3d4c") || strings.Contains(raw, planted) || !strings.Contains(raw, "****") {
		t.Fatalf("line %s", raw)
	}
}

func TestABodyOverTheLimitLeavesNoFragmentInTheRecord(t *testing.T) {
	r := startRig(t, http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		io.Copy(io.Discard, req.Body)
	}), nil)
	body := planted + strings.Repeat("x", wire.MaxBody)
	res, err := r.client.Post(r.url("/big"), "text/plain", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	drain(res)
	got := r.take(InitialTake)
	if strings.Contains(got.raw, planted) || !strings.Contains(got.raw, string(wire.OmitTooLarge)) || got.end.BodiesOmitted != 1 {
		t.Fatalf("end %+v, line of %d bytes", got.end, len(got.raw))
	}
}

func TestABodyThatBrokeOffLeavesNoFragmentInTheRecord(t *testing.T) {
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			panic(err)
		}
		defer conn.Close()
		io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 500\r\nContent-Type: text/plain\r\n\r\n"+planted)
	}), nil)
	res, err := r.client.Get(r.url("/cut"))
	if err != nil {
		t.Fatal(err)
	}
	drain(res)
	got := r.take(InitialTake)
	if strings.Contains(got.raw, planted) || len(got.calls) != 1 || got.calls[0].Response.Size != int64(len(planted)) {
		t.Fatalf("take %s", got.raw)
	}
}

func TestDiagnosticsNameNoReceivedValue(t *testing.T) {
	var logged bytes.Buffer
	var mu lockedWriter
	mu.w = &logged
	r := startRig(t, okUpstream(), func(o *Options) { o.Log = slog.New(slog.NewJSONHandler(&mu, nil)) })
	r.get("/" + planted + "?token=" + planted)
	r.take(InitialTake)
	text := mu.String()
	if strings.Contains(text, planted) || !strings.Contains(text, "wire take written") {
		t.Fatalf("log %q", text)
	}
}

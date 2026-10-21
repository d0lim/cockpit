package api

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var screenMutations = []porttest.Mutation{
	{"the screen sits behind the host guard", "mount the screen on an outer mux beside Guard instead of inside it", "red: a foreign Host header gets the index", "TestTheScreenIsServedAtTheRootBehindTheHostGuard"},
	{"unknown api paths stay JSON problems", "drop the /api/ fallback so the screen answers", "red: /api/unknown answers the screen's page", "TestTheScreenIsServedAtTheRootBehindTheHostGuard"},
	{"screen writes pass the write guard", "mount the screen beside Guard for POST", "red: a POST without Origin reaches the screen", "TestAScreenWriteReachesTheScreenOnlyWithTheWriteGuardHeaders"},
	{"no screen means not found", "serve an empty handler when Screen is nil", "red: / answers 200 with an empty body", "TestWithoutAScreenTheRootAnswersNotFound"},
}

func TestScreenMutationRecordMatchesThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "screen_test.go", screenMutations)
}

const screenIndex = "<!doctype html><p>runs</p>"

type stubScreen struct{ writes atomic.Int32 }

func (s *stubScreen) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.writes.Add(1)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, screenIndex)
}

func (f *fixture) getHost(path, host string) (int, http.Header, string) {
	f.t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.srv.URL+path, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	req.Host = host
	res, err := f.srv.Client().Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, string(raw)
}

func TestTheScreenIsServedAtTheRootBehindTheHostGuard(t *testing.T) {
	f := newFixture(t, func(d *Deps) { d.Screen = &stubScreen{} })
	loopback := "127.0.0.1:" + itoa(f.port)
	if status, _, body := f.getHost("/", loopback); status != http.StatusOK || body != screenIndex {
		t.Fatalf("index %d %q", status, body)
	}
	if status, _, body := f.getHost("/runs/x", "localhost:"+itoa(f.port)); status != http.StatusOK || body != screenIndex {
		t.Fatalf("screen path %d %q", status, body)
	}
	if status, _, body := f.getHost("/", "rebind.example:"+itoa(f.port)); status != http.StatusForbidden || !strings.Contains(body, "forbidden:host") || strings.Contains(body, "doctype") {
		t.Fatalf("foreign host %d %q", status, body)
	}
	if status, _, body := f.getHost("/api/unknown", loopback); status != http.StatusNotFound || !strings.Contains(body, "NOT_FOUND") || strings.Contains(body, "doctype") {
		t.Fatalf("unknown api path %d %q", status, body)
	}
}

func TestAScreenWriteReachesTheScreenOnlyWithTheWriteGuardHeaders(t *testing.T) {
	stub := &stubScreen{}
	f := newFixture(t, func(d *Deps) { d.Screen = stub })
	paths := []string{"/runs/" + rid(1) + "/do/keep", "/runs/submit"}
	denied := map[string]http.Header{
		"no origin":      {HeaderCockpit: {"1"}},
		"foreign origin": {"Origin": {"http://evil.example"}, HeaderCockpit: {"1"}},
		"no header":      {"Origin": {f.origin}},
	}
	tried := 0
	for _, path := range paths {
		for name, h := range denied {
			tried++
			if status, raw := f.do(http.MethodPost, path, nil, h); status != http.StatusForbidden {
				t.Errorf("%s %s: %d %s", path, name, status, raw)
			}
		}
	}
	if stub.writes.Load() != 0 || tried != 6 {
		t.Fatalf("%d writes reached the screen after %d tries", stub.writes.Load(), tried)
	}
	for i, path := range paths {
		if status, raw := f.do(http.MethodPost, path, nil, f.cli()); status != http.StatusAccepted || stub.writes.Load() != int32(i+1) {
			t.Fatalf("guarded write to %s: %d %s, %d writes", path, status, raw, stub.writes.Load())
		}
	}
}

func TestWithoutAScreenTheRootAnswersNotFound(t *testing.T) {
	f := newFixture(t, nil)
	status, raw := f.get("/")
	if status != http.StatusNotFound || !strings.Contains(string(raw), "NOT_FOUND") {
		t.Fatalf("root %d %s", status, raw)
	}
	if status, _ := f.get("/api/runs"); status != http.StatusOK {
		t.Fatalf("canary: the list answers %d", status)
	}
}

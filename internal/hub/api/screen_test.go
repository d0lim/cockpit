package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var screenMutations = []porttest.Mutation{
	{"the screen sits behind the host guard", "mount the screen on an outer mux beside Guard instead of inside it", "red: a foreign Host header gets the index", "TestTheScreenIsServedAtTheRootBehindTheHostGuard"},
	{"unknown api paths stay JSON problems", "drop the /api/ fallback so the file server answers", "red: /api/unknown answers a plain text 404", "TestTheScreenIsServedAtTheRootBehindTheHostGuard"},
	{"writes to the screen are refused", "drop the method check in screen", "red: a POST to / is served", "TestTheScreenIsServedAtTheRootBehindTheHostGuard"},
	{"no screen means not found", "serve an empty file server when Screen is nil", "red: / answers a directory listing", "TestWithoutAScreenTheRootAnswersNotFound"},
}

func TestScreenMutationRecordMatchesThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "screen_test.go", screenMutations)
}

const screenIndex = "<!doctype html><script src=\"./app.js\"></script>"

func screenFiles() fstest.MapFS {
	return fstest.MapFS{"index.html": {Data: []byte(screenIndex)}, "app.js": {Data: []byte("console.info(1)")}}
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
	f := newFixture(t, func(d *Deps) { d.Screen = screenFiles() })
	loopback := "127.0.0.1:" + itoa(f.port)
	status, header, body := f.getHost("/", loopback)
	if status != http.StatusOK || body != screenIndex || header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("index %d %q %v", status, body, header)
	}
	if status, header, body := f.getHost("/app.js", "localhost:"+itoa(f.port)); status != http.StatusOK || !strings.Contains(header.Get("Content-Type"), "javascript") || body != "console.info(1)" {
		t.Fatalf("asset %d %v %q", status, header, body)
	}
	if status, _, body := f.getHost("/", "rebind.example:"+itoa(f.port)); status != http.StatusForbidden || !strings.Contains(body, "forbidden:host") || strings.Contains(body, "doctype") {
		t.Fatalf("foreign host %d %q", status, body)
	}
	status, raw := f.post("/", nil)
	if status != http.StatusNotFound || !strings.Contains(string(raw), "NOT_FOUND") {
		t.Fatalf("post to the screen %d %s", status, raw)
	}
	if status, _, body := f.getHost("/api/unknown", loopback); status != http.StatusNotFound || !strings.Contains(body, "NOT_FOUND") || strings.Contains(body, "doctype") {
		t.Fatalf("unknown api path %d %q", status, body)
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

package recorder

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/wire"
)

const planted = "PLANTED-secret-7f3a9c"

const testProject = "run-1"

func testLimits() Limits {
	l := DefaultLimits()
	l.Quiet = 30 * time.Millisecond
	l.MaxWait = 3 * time.Second
	return l
}

func testMasker(t *testing.T, fields ...mask.PublicField) *mask.Masker {
	t.Helper()
	base, err := mask.New(mask.Options{Mode: mask.Runner})
	if err != nil {
		t.Fatal(err)
	}
	m, err := base.WithPublicFields(fields)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

type rig struct {
	t        *testing.T
	rec      *Recorder
	upstream string
	client   *http.Client
}

func portOf(t *testing.T, addr string) int {
	t.Helper()
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func startRig(t *testing.T, handler http.Handler, tune func(*Options)) *rig {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return startRigAt(t, srv.Listener.Addr().String(), tune)
}

func startRigAt(t *testing.T, upstream string, tune func(*Options)) *rig {
	t.Helper()
	opts := Options{
		Project: testProject,
		Routes:  []Route{{Service: "127.0.0.1", Port: portOf(t, upstream)}},
		Masker:  testMasker(t),
		Limits:  testLimits(),
		LookupAddr: func(context.Context, string) ([]string, error) {
			return []string{testProject + "-orders-1." + testProject + "_default."}, nil
		},
	}
	if tune != nil {
		tune(&opts)
	}
	rec, err := Listen(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- rec.Serve(ctx) }()
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	t.Cleanup(func() {
		client.CloseIdleConnections()
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve returned %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("the recorder did not stop")
		}
	})
	return &rig{t: t, rec: rec, upstream: upstream, client: client}
}

func (r *rig) addr() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(portOf(r.t, r.rec.RouteAddr(0))))
}

func (r *rig) url(path string) string { return "http://" + r.addr() + path }

func (r *rig) get(path string) {
	r.t.Helper()
	res, err := r.client.Get(r.url(path))
	if err != nil {
		r.t.Fatal(err)
	}
	drain(res)
}

func drain(res *http.Response) {
	var sink bytes.Buffer
	sink.ReadFrom(res.Body)
	res.Body.Close()
}

func (r *rig) control(request string) (string, error) {
	r.t.Helper()
	var out bytes.Buffer
	err := Control(context.Background(), r.rec.ControlAddr(), request, &out)
	return out.String(), err
}

type taken struct {
	raw   string
	calls []wire.Call
	end   wire.End
}

func parseTake(t *testing.T, raw string) taken {
	t.Helper()
	got := taken{raw: raw}
	ends := 0
	for _, text := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		var l wire.Line
		dec := json.NewDecoder(strings.NewReader(text))
		dec.UseNumber()
		if err := dec.Decode(&l); err != nil {
			t.Fatalf("line %q: %v", text, err)
		}
		switch {
		case l.Call != nil && ends == 0:
			got.calls = append(got.calls, *l.Call)
		case l.End != nil:
			got.end = *l.End
			ends++
		default:
			t.Fatalf("unexpected line %q", text)
		}
	}
	if ends != 1 {
		t.Fatalf("%d end lines in %q", ends, raw)
	}
	return got
}

func (r *rig) take(name string) taken {
	r.t.Helper()
	raw, err := r.control("take " + name)
	if err != nil {
		r.t.Fatal(err)
	}
	return parseTake(r.t, raw)
}

func (r *rig) onlyCall() wire.Call {
	r.t.Helper()
	got := r.take(InitialTake)
	if len(got.calls) != 1 {
		r.t.Fatalf("%d calls in %s", len(got.calls), got.raw)
	}
	return got.calls[0]
}

type lockedWriter struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func (l *lockedWriter) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.String()
}

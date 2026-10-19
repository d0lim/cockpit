package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/d0lim/cockpit/internal/hub/hubtest"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/sse"
	"github.com/d0lim/cockpit/internal/hub/store"
)

const planted = "PLANTED-api-secret-41c7e2"

type fixture struct {
	t      *testing.T
	srv    *httptest.Server
	port   int
	origin string
	world  *hubtest.World
	sched  *scheduler.Scheduler
	st     *store.Store
	bus    *sse.Bus
}

func newFixture(t *testing.T, edit func(*Deps)) *fixture {
	t.Helper()
	st := store.Open(filepath.Join(t.TempDir(), "home"))
	w := hubtest.New(st)
	sched := scheduler.New(w.Deps())
	t.Cleanup(func() { sched.Dispose(); sched.Idle() })
	bus := sse.New("1000", 0, nil)
	f := &fixture{t: t, world: w, sched: sched, st: st, bus: bus}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.port = ln.Addr().(*net.TCPAddr).Port
	f.origin = "http://127.0.0.1:" + strconv.Itoa(f.port)
	d := Deps{Port: f.port, Store: st, Scheduler: sched, Stream: &sse.Stream{Bus: bus, Ping: time.Hour, Poll: 10 * time.Millisecond}}
	if edit != nil {
		edit(&d)
	}
	f.srv = &httptest.Server{Listener: ln, Config: &http.Server{Handler: Handler(d)}}
	f.srv.Start()
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fixture) do(method, path string, body any, header http.Header) (int, []byte) {
	f.t.Helper()
	var payload io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, f.srv.URL+path, payload)
	if err != nil {
		f.t.Fatal(err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	res, err := f.srv.Client().Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, raw
}

func (f *fixture) cli() http.Header {
	return http.Header{"Origin": {f.origin}, HeaderCockpit: {"1"}, "Content-Type": {"application/json"}}
}

func (f *fixture) post(path string, body any) (int, []byte) {
	return f.do(http.MethodPost, path, body, f.cli())
}

func (f *fixture) get(path string) (int, []byte) {
	return f.do(http.MethodGet, path, nil, nil)
}

func (f *fixture) seed(m store.Meta) store.Meta {
	f.t.Helper()
	if err := f.st.WriteMeta(m); err != nil {
		f.t.Fatal(err)
	}
	list, _ := f.st.ListRuns()
	f.st.WriteState(store.RebuildState(list.Runs))
	return m
}

func (f *fixture) runDir(id string) string {
	dir, err := f.st.RunDir(id)
	if err != nil {
		f.t.Fatal(err)
	}
	return dir
}

func rid(n int) string { return "run-20261019-0100" + strconv.Itoa(10+n) + "-abc1234" }

func runMeta(n int, state lifecycle.State, edit ...func(*store.Meta)) store.Meta {
	m := store.Meta{State: state, ID: rid(n), SHA: hubtest.ToySHA, Suite: "toy", HostAPI: 1, CockpitVersion: "0.1.0", Submitter: "cli", CreatedAt: "2026-10-19T01:00:00.000Z"}
	for _, e := range edit {
		e(&m)
	}
	return m
}

func doneUp(m *store.Meta) {
	slot := 1
	m.Slot, m.StartedAt, m.FinishedAt, m.LastUpAt = &slot, "2026-10-19T01:00:01.000Z", "2026-10-19T01:00:09.000Z", "2026-10-19T01:00:09.000Z"
}

func schemaFor(t *testing.T, typ reflect.Type) *jsonschema.Resolved {
	t.Helper()
	nullable := &jsonschema.Schema{Types: []string{"string", "null"}}
	s, err := jsonschema.ForType(typ, &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[lifecycle.Verdict](): nullable,
		reflect.TypeFor[json.RawMessage]():   {},
		reflect.TypeFor[map[string]any]():    {Type: "object"},
		reflect.TypeFor[map[string]int64]():  {Type: "object"},
		reflect.TypeFor[map[string]string](): {Type: "object"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func validate(t *testing.T, typ reflect.Type, raw []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("not a JSON object: %s", raw)
	}
	if err := schemaFor(t, typ).Validate(doc); err != nil {
		t.Fatalf("%s does not match %s: %v\n%s", typ.Name(), typ, err, raw)
	}
	return doc
}

func until(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 5s")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

package screen

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/a-h/templ"

	"github.com/d0lim/cockpit/internal/hub/hubtest"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/sse"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ui"
)

type fixture struct {
	t     *testing.T
	st    *store.Store
	sched *scheduler.Scheduler
	sc    *Screen
	srv   *httptest.Server
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st := store.Open(filepath.Join(t.TempDir(), "home"))
	w := hubtest.New(st)
	sched := scheduler.New(w.Deps())
	t.Cleanup(func() { sched.Dispose(); sched.Idle() })
	sc := New(Deps{Scheduler: sched, Bus: sse.New("1", 0, nil), Assets: ui.Files(), Identity: Identity{Home: st.Home(), Port: 7710}})
	srv := httptest.NewServer(sc.Handler())
	t.Cleanup(srv.Close)
	return &fixture{t: t, st: st, sched: sched, sc: sc, srv: srv}
}

func (f *fixture) seed(m store.Meta) {
	f.t.Helper()
	if err := f.st.WriteMeta(m); err != nil {
		f.t.Fatal(err)
	}
	list, _ := f.st.ListRuns()
	f.st.WriteState(store.RebuildState(list.Runs))
}

func (f *fixture) get(path string) (int, http.Header, string) {
	f.t.Helper()
	res, err := http.Get(f.srv.URL + path)
	if err != nil {
		f.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, string(raw)
}

func (f *fixture) state(path string) *state {
	f.t.Helper()
	st, err := f.sc.load(context.Background(), path)
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var out bytes.Buffer
	if err := c.Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func rid(n int) string { return "run-20261019-0100" + strconv.Itoa(10+n) + "-abc1234" }

func runMeta(n int, state lifecycle.State, edit ...func(*store.Meta)) store.Meta {
	m := store.Meta{State: state, ID: rid(n), SHA: hubtest.ToySHA, Suite: "toy", HostAPI: 1, CockpitVersion: "0.1.0", Submitter: "cli", CreatedAt: "2026-10-19T01:00:0" + strconv.Itoa(n) + ".000Z"}
	for _, e := range edit {
		e(&m)
	}
	return m
}

func doneUp(m *store.Meta) {
	slot := 1
	m.Slot, m.StartedAt, m.FinishedAt, m.LastUpAt = &slot, "2026-10-19T01:00:01.000Z", "2026-10-19T01:00:09.000Z", "2026-10-19T01:00:09.000Z"
}

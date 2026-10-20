package screen

//go:generate go tool templ generate -path . -include-version=false
//go:generate go run ../../../scripts/stripcomments .

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/labels"
	"github.com/d0lim/cockpit/internal/hub/live"
	"github.com/d0lim/cockpit/internal/hub/registry"
	"github.com/d0lim/cockpit/internal/hub/sse"
	"github.com/d0lim/cockpit/internal/version"
)

var labelTable = labels.Load()

const csp = "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

type Identity struct {
	Home string
	Port int
}

type Deps struct {
	Scheduler api.Scheduler
	Hubs      func() []registry.Hub
	Bus       *sse.Bus
	Signals   sse.Signals
	Assets    fs.FS
	Identity  Identity
	Log       *slog.Logger
}

type Screen struct {
	d Deps
}

func New(d Deps) *Screen {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Screen{d: d}
}

type state struct {
	ctx  context.Context
	d    Deps
	path string
	list api.HubList
	run  *api.RunItem
}

func (s *Screen) load(ctx context.Context, path string) (*state, error) {
	l, err := s.d.Scheduler.List()
	if err != nil {
		return nil, err
	}
	var hubs []registry.Hub
	if s.d.Hubs != nil {
		hubs = s.d.Hubs()
	}
	return &state{ctx: ctx, d: s.d, path: path, list: api.PublicList(l, hubs)}, nil
}

func (st *state) self() api.HubItem {
	for _, h := range st.list.Hubs {
		if h.Self {
			return h
		}
	}
	return api.HubItem{Name: "this hub", Self: true, Up: true}
}

func (st *state) version() string { return version.String() }

var errNoScreen = errors.New("no such screen")

func (s *Screen) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, v := range views {
		mux.HandleFunc("GET "+v.pattern, func(w http.ResponseWriter, r *http.Request) { s.page(w, r, v) })
	}
	if s.d.Assets != nil {
		files := http.StripPrefix("/assets/", http.FileServerFS(s.d.Assets))
		mux.HandleFunc("GET /assets/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			files.ServeHTTP(w, r)
		})
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.problem(w, r, http.StatusNotFound, "There is no screen at "+r.URL.Path+".")
	})
	return mux
}

func (s *Screen) Live() http.Handler {
	return &live.Handler{Bus: s.d.Bus, Signals: s.d.Signals, Resolve: s.Resolve, Log: s.d.Log}
}

func headers(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", csp)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
}

func (s *Screen) page(w http.ResponseWriter, r *http.Request, v view) {
	st, err := s.load(r.Context(), r.URL.Path)
	if err != nil {
		s.problem(w, r, http.StatusInternalServerError, "The run list could not be read.")
		return
	}
	if v.run {
		if status, msg := st.pickRun(r.PathValue("id")); status != http.StatusOK {
			s.problem(w, r, status, msg)
			return
		}
	}
	headers(w)
	if err := layout(st, v.title, v.body(st)).Render(r.Context(), w); err != nil {
		s.d.Log.Warn("screen render failed", "path", r.URL.Path, "error", err)
	}
}

func (s *Screen) problem(w http.ResponseWriter, r *http.Request, status int, message string) {
	headers(w)
	w.WriteHeader(status)
	st, err := s.load(r.Context(), r.URL.Path)
	if err != nil {
		st = &state{ctx: r.Context(), d: s.d, path: r.URL.Path}
	}
	layout(st, "Not here", problemBody(status, message)).Render(r.Context(), w)
}

func (s *Screen) Resolve(ctx context.Context, path string) (*live.Screen, error) {
	v, values, ok := match(path)
	if !ok {
		return nil, errNoScreen
	}
	if v.run {
		st, err := s.load(ctx, path)
		if err != nil {
			return nil, err
		}
		if status, _ := st.pickRun(values["id"]); status != http.StatusOK {
			return nil, errNoScreen
		}
	}
	regions := v.regions()
	touches := map[sse.Kind][]string{}
	for _, reg := range regions {
		for _, k := range reg.kinds {
			touches[k] = append(touches[k], reg.id)
		}
	}
	sc := &live.Screen{Run: values["id"], Touches: touches}
	for _, reg := range regions {
		sc.Regions = append(sc.Regions, reg.id)
		if reg.signals {
			sc.Signals = append(sc.Signals, reg.id)
		}
	}
	sc.Render = func(ctx context.Context, ids []string) ([]byte, error) {
		return s.renderRegions(ctx, path, v, values, ids)
	}
	return sc, nil
}

func (s *Screen) renderRegions(ctx context.Context, path string, v view, values map[string]string, ids []string) ([]byte, error) {
	st, err := s.load(ctx, path)
	if err != nil {
		return nil, err
	}
	if v.run {
		if status, _ := st.pickRun(values["id"]); status != http.StatusOK {
			return nil, live.ErrGone
		}
	}
	var out bytes.Buffer
	for _, reg := range v.regions() {
		if !contains(ids, reg.id) {
			continue
		}
		if err := partial(reg.id, reg.render(st)).Render(ctx, &out); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

func contains(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func match(path string) (view, map[string]string, bool) {
	if !strings.HasPrefix(path, "/") {
		return view{}, nil, false
	}
	segs := strings.Split(strings.Trim(path, "/"), "/")
	for _, v := range views {
		want := strings.Split(strings.Trim(v.pattern, "/"), "/")
		if v.pattern == "/{$}" {
			want = []string{""}
		}
		if len(want) != len(segs) {
			continue
		}
		values := map[string]string{}
		ok := true
		for i, w := range want {
			if strings.HasPrefix(w, "{") {
				values[strings.Trim(w, "{}")] = segs[i]
				continue
			}
			if w != segs[i] {
				ok = false
				break
			}
		}
		if ok {
			return v, values, true
		}
	}
	return view{}, nil, false
}

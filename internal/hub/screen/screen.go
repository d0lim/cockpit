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
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/labels"
	"github.com/d0lim/cockpit/internal/hub/live"
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
	Reader   *api.Reader
	Bus      *sse.Bus
	Assets   fs.FS
	Identity Identity
	Log      *slog.Logger
}

type Screen struct {
	d       Deps
	signals *signalCache
}

func New(d Deps) *Screen {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Screen{d: d, signals: newSignalCache(d.Reader)}
}

type state struct {
	ctx    context.Context
	s      *Screen
	path   string
	query  url.Values
	values map[string]string
	list   api.HubList
	run    *api.RunItem
	tab    string

	services map[string]serviceState
}

func (s *Screen) load(ctx context.Context, path string, query url.Values) (*state, error) {
	l, err := s.d.Reader.List()
	if err != nil {
		return nil, err
	}
	if query == nil {
		query = url.Values{}
	}
	return &state{ctx: ctx, s: s, path: path, query: query, values: map[string]string{}, list: l}, nil
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

func (st *state) screenParam() string {
	if len(st.query) == 0 {
		return st.path
	}
	return st.path + "?" + st.query.Encode()
}

func itoa(n int) string { return strconv.Itoa(n) }

var errNoScreen = errors.New("no such screen")

func (s *Screen) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, v := range views {
		mux.HandleFunc("GET "+v.pattern, func(w http.ResponseWriter, r *http.Request) { s.page(w, r, v) })
	}
	mux.HandleFunc("POST /runs/{id}/do/{op}", s.act)
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
	return &live.Handler{Bus: s.d.Bus, Signals: s.d.Reader.SignalSource(), Resolve: s.Resolve, Log: s.d.Log}
}

func headers(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", csp)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
}

func (s *Screen) page(w http.ResponseWriter, r *http.Request, v view) {
	st, err := s.load(r.Context(), r.URL.Path, r.URL.Query())
	if err != nil {
		s.problem(w, r, http.StatusInternalServerError, "The run list could not be read.")
		return
	}
	_, st.values, _ = match(r.URL.Path)
	if status, msg := st.prepare(v); status != http.StatusOK {
		s.problem(w, r, status, msg)
		return
	}
	headers(w)
	if err := layout(st, v.titleOf(st), v.body(st)).Render(r.Context(), w); err != nil {
		s.d.Log.Warn("screen render failed", "path", r.URL.Path, "error", err)
	}
}

func (s *Screen) problem(w http.ResponseWriter, r *http.Request, status int, message string) {
	headers(w)
	w.WriteHeader(status)
	st, err := s.load(r.Context(), r.URL.Path, nil)
	if err != nil {
		st = &state{ctx: r.Context(), s: s, path: r.URL.Path, query: url.Values{}}
	}
	layout(st, "Not here", problemBody(status, message)).Render(r.Context(), w)
}

func (st *state) prepare(v view) (int, string) {
	st.tab = v.tab
	if !v.run {
		return http.StatusOK, ""
	}
	if status, msg := st.pickRun(st.values["id"]); status != http.StatusOK {
		return status, msg
	}
	if v.served && !served(*st.run, st.list) {
		return http.StatusNotFound, "This hub does not serve host API " + itoa(st.run.HostAPI) + "; only the overview and the pipeline of this run are shown."
	}
	if v.check != nil {
		return v.check(st)
	}
	return http.StatusOK, ""
}

func splitScreen(raw string) (string, url.Values) {
	path, rawQuery, _ := strings.Cut(raw, "?")
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		q = url.Values{}
	}
	return path, q
}

func (s *Screen) Resolve(ctx context.Context, raw string) (*live.Screen, error) {
	path, query := splitScreen(raw)
	v, values, ok := match(path)
	if !ok {
		return nil, errNoScreen
	}
	st, err := s.load(ctx, path, query)
	if err != nil {
		return nil, err
	}
	st.values = values
	if status, _ := st.prepare(v); status != http.StatusOK {
		return nil, errNoScreen
	}
	sc := &live.Screen{Touches: map[sse.Kind][]string{}}
	if v.run {
		sc.Run = values["id"]
	}
	for _, reg := range v.regions() {
		sc.Regions = append(sc.Regions, reg.id)
		for _, k := range reg.kinds {
			sc.Touches[k] = append(sc.Touches[k], reg.id)
		}
		if reg.signals {
			sc.Signals = append(sc.Signals, reg.id)
		}
	}
	sc.Render = func(ctx context.Context, ids []string) ([]byte, error) {
		return s.renderRegions(ctx, path, query, v, values, ids)
	}
	return sc, nil
}

func (s *Screen) renderRegions(ctx context.Context, path string, query url.Values, v view, values map[string]string, ids []string) ([]byte, error) {
	st, err := s.load(ctx, path, query)
	if err != nil {
		return nil, err
	}
	st.values = values
	if status, _ := st.prepare(v); status != http.StatusOK {
		return nil, live.ErrGone
	}
	var out bytes.Buffer
	for _, reg := range v.regions() {
		if !slices.Contains(ids, reg.id) {
			continue
		}
		if err := partial(reg.id, reg.render(st)).Render(ctx, &out); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
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

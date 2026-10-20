package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/registry"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/sse"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/mask"
)

type Scheduler interface {
	Submit(ctx context.Context, in scheduler.SubmitInput) scheduler.SubmitResult
	Keep(id string, a scheduler.Actor) ticker.Result
	Unkeep(id string, a scheduler.Actor) ticker.Result
	Stop(id string, a scheduler.Actor) ticker.Result
	Resume(id string, a scheduler.Actor) ticker.Result
	Destroy(id string, a scheduler.Actor) ticker.Result
	Abort(id string, a scheduler.Actor) ticker.Result
	Exec(id, flow string, a scheduler.Actor) scheduler.ExecResponse
	List() (scheduler.HubList, error)
	Get(id string) *scheduler.RunView
	AllowedEvents(m store.Meta) []lifecycle.Event
}

type CollectorState struct {
	ID       string        `json:"id"`
	State    collect.State `json:"state"`
	LastOKAt string        `json:"lastOkAt,omitempty"`
	Failures int           `json:"failures"`
	Reason   string        `json:"reason,omitempty"`
}

type Collectors interface {
	Status(runID string) []CollectorState
}

type Deps struct {
	Port       int
	Store      *store.Store
	Scheduler  Scheduler
	Stream     *sse.Stream
	Signals    sse.Signals
	Collectors Collectors
	Services   func(m store.Meta) []ServicePort
	Suites     func(ctx context.Context, worktree string) ([]string, error)
	Mask       func(runID string) *mask.Masker
	Hubs       func() []registry.Hub
	Changed    func()
	Screen     http.Handler
	Live       http.Handler
	Log        *slog.Logger
}

type api struct{ d Deps }

func (a *api) log() *slog.Logger {
	if a.d.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return a.d.Log
}

type routeSpec struct {
	Method  string
	Pattern string
	handle  func(w http.ResponseWriter, r *http.Request) error
}

func (a *api) routes() []routeSpec {
	return []routeSpec{
		{http.MethodGet, "/api/events", a.events},
		{http.MethodGet, "/api/suites", a.suites},
		{http.MethodGet, "/api/runs", a.list},
		{http.MethodPost, "/api/runs", a.submit},
		{http.MethodGet, "/api/runs/{id}", a.get},
		{http.MethodPost, "/api/runs/{id}/{op}", a.operate},
		{http.MethodGet, "/api/runs/{id}/report", a.report},
		{http.MethodGet, "/api/runs/{id}/kit", a.kit},
		{http.MethodGet, "/api/runs/{id}/flows/{flow}", a.flow},
		{http.MethodGet, "/api/runs/{id}/source", a.source},
		{http.MethodGet, "/api/runs/{id}/events", a.runEvents},
		{http.MethodGet, "/api/runs/{id}/signals", a.signals},
	}
}

func Routes() []string {
	var out []string
	for _, r := range (&api{}).routes() {
		out = append(out, r.Method+" "+r.Pattern)
	}
	return out
}

func Handler(d Deps) http.Handler {
	a := &api{d: d}
	byPattern := map[string]map[string]func(w http.ResponseWriter, r *http.Request) error{}
	var order []string
	for _, r := range a.routes() {
		if byPattern[r.Pattern] == nil {
			byPattern[r.Pattern] = map[string]func(w http.ResponseWriter, r *http.Request) error{}
			order = append(order, r.Pattern)
		}
		byPattern[r.Pattern][r.Method] = r.handle
	}
	mux := http.NewServeMux()
	for _, pattern := range order {
		methods := byPattern[pattern]
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			h, ok := methods[r.Method]
			if !ok {
				writeProblem(a.log(), w, notFound("not found"))
				return
			}
			if err := h(w, r); err != nil {
				writeProblem(a.log(), w, err)
			}
		})
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) { writeProblem(a.log(), w, notFound("not found")) })
	if d.Live != nil {
		mux.Handle("GET /live", d.Live)
	}
	mux.Handle("/", screen(d.Screen, a.log()))
	return Guard(d.Port, a.log(), mux)
}

func (a *api) hubs() []registry.Hub {
	if a.d.Hubs == nil {
		return nil
	}
	return a.d.Hubs()
}

func (a *api) changed() {
	if a.d.Changed != nil {
		a.d.Changed()
	}
}

func (a *api) reader() *Reader { return &Reader{a: a} }

func (a *api) runOf(r *http.Request) (*scheduler.RunView, error) {
	return a.runByID(r.PathValue("id"))
}

func (a *api) runByID(id string) (*scheduler.RunView, error) {
	if !store.IsRunID(id) {
		return nil, badRequest("not a run id: run-<8-digit date>-<6-digit time>-<7-character commit>")
	}
	v := a.d.Scheduler.Get(id)
	if v == nil {
		return nil, notFound("no such run: " + id)
	}
	return v, nil
}

func (a *api) runDir(id string) (string, error) {
	return a.d.Store.RunDir(id)
}

func (a *api) events(w http.ResponseWriter, r *http.Request) error {
	target := r.URL.Query().Get("target")
	if target == "" {
		target = sse.HubTarget
	}
	if target != sse.HubTarget {
		if !store.IsRunID(target) {
			return &Problem{Status: http.StatusBadRequest, Code: CodeBadTarget, Message: "target is hub or a run id"}
		}
		if a.d.Scheduler.Get(target) == nil {
			return notFound("no such run: " + target)
		}
	}
	a.d.Stream.Serve(w, r, target, target != sse.HubTarget)
	return nil
}

func (a *api) runEvents(w http.ResponseWriter, r *http.Request) error {
	v, err := a.runOf(r)
	if err != nil {
		return err
	}
	a.d.Stream.Serve(w, r, v.ID, true)
	return nil
}

type SuitesView struct {
	Worktree string      `json:"worktree"`
	Suites   []SuiteName `json:"suites"`
}

type SuiteName struct {
	Name string `json:"name"`
}

func (a *api) suites(w http.ResponseWriter, r *http.Request) error {
	worktree := r.URL.Query().Get("worktree")
	view := SuitesView{Worktree: worktree, Suites: []SuiteName{}}
	if a.d.Suites != nil && worktree != "" {
		names, err := a.d.Suites(r.Context(), worktree)
		if err != nil {
			return badRequest(err.Error())
		}
		for _, n := range names {
			view.Suites = append(view.Suites, SuiteName{Name: n})
		}
	}
	writeJSON(a.log(), w, http.StatusOK, view)
	return nil
}

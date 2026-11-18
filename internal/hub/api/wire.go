package api

import (
	"errors"
	"io/fs"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/wire"
)

const (
	WirePending      = "pending"
	WireReady        = "ready"
	WireNone         = "none"
	WireScopeStep    = "step"
	WireScopeOutside = "outside"
	WirePageSize     = 200
	wirePageMax      = 500
)

type WireQuery struct {
	Take, Flow, Step, Scope, Target string
	Offset, Limit                   int
}

type WireStanding struct {
	Status    string             `json:"status"`
	State     contract.WireState `json:"state,omitempty"`
	Detail    string             `json:"detail,omitempty"`
	Discarded bool               `json:"discarded,omitempty"`
	Takes     []string           `json:"takes"`
}

type WireStep struct {
	Flow   string              `json:"flow"`
	Step   string              `json:"step"`
	Title  string              `json:"title,omitempty"`
	Status contract.StepStatus `json:"status,omitempty"`
	Exact  int                 `json:"exact"`
	Nested int                 `json:"nested"`
	Time   int                 `json:"time"`
}

func (s WireStep) Calls() int { return s.Exact + s.Nested + s.Time }

type WireRouteCalls struct {
	Name    string `json:"name"`
	Service string `json:"service"`
	Port    int    `json:"port"`
	Calls   int    `json:"calls"`
}

type WireHead struct {
	Seq        int64         `json:"seq"`
	Caller     string        `json:"caller"`
	Target     string        `json:"target"`
	Port       int           `json:"port"`
	Method     string        `json:"method"`
	Path       string        `json:"path,omitempty"`
	Status     int           `json:"status"`
	Outcome    wire.Outcome  `json:"outcome"`
	Withheld   bool          `json:"withheld,omitempty"`
	Started    time.Time     `json:"started"`
	DurationMs int64         `json:"durationMs"`
	Link       wire.LinkKind `json:"link"`
	Parent     int64         `json:"parent,omitempty"`
	Depth      int           `json:"depth,omitempty"`
}

type WireView struct {
	ID string `json:"id"`
	WireStanding
	Take    string               `json:"take,omitempty"`
	Record  *contract.WireStatus `json:"record,omitempty"`
	Steps   []WireStep           `json:"steps"`
	Outside int                  `json:"outside"`
	Routes  []WireRouteCalls     `json:"routes"`
	Scope   string               `json:"scope,omitempty"`
	Flow    string               `json:"flow,omitempty"`
	Step    string               `json:"step,omitempty"`
	Calls   []WireHead           `json:"calls"`
	Total   int                  `json:"total"`
	Offset  int                  `json:"offset"`
	Limit   int                  `json:"limit"`
}

type WireCallView struct {
	ID   string    `json:"id"`
	Take string    `json:"take"`
	Head WireHead  `json:"head"`
	Flow string    `json:"flow,omitempty"`
	Step string    `json:"step,omitempty"`
	Why  string    `json:"why"`
	Call wire.Call `json:"call"`
}

type wireRun struct {
	view  *scheduler.RunView
	dir   string
	store wire.Store
	mask  *wireMask
}

type wireMask struct {
	m    *mask.Masker
	seen map[string]string
}

func (w *wireMask) text(s string) string {
	if out, ok := w.seen[s]; ok {
		return out
	}
	out := w.m.Mask(s)
	w.seen[s] = out
	return out
}

func (w *wireMask) problem(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return notFound("this run has no such take or call")
	}
	return &Problem{Status: http.StatusInternalServerError, Code: CodeInternal, Message: w.m.Mask(err.Error())}
}

func (r *Reader) wireRun(id string) (wireRun, error) {
	v, dir, err := r.run(id)
	if err != nil {
		return wireRun{}, err
	}
	if !v.Meta.Wire {
		return wireRun{}, notFound("this run does not record its service calls")
	}
	if r.a.d.Mask == nil {
		return wireRun{}, &Problem{Status: http.StatusConflict, Code: CodeNotAvailable, Message: "no run masker: the wire record is not served"}
	}
	return wireRun{view: v, dir: dir, store: wire.OpenStore(dir), mask: &wireMask{m: r.a.d.Mask(v.ID), seen: map[string]string{}}}, nil
}

func (r *Reader) WireStanding(id string) (WireStanding, error) {
	run, err := r.wireRun(id)
	if err != nil {
		return WireStanding{}, err
	}
	return r.wireStanding(run)
}

func (r *Reader) wireStanding(run wireRun) (WireStanding, error) {
	takes, err := run.store.Takes()
	if err != nil {
		return WireStanding{}, run.mask.problem(err)
	}
	m := run.view.Meta
	s := WireStanding{Status: WireReady, Takes: takes, Discarded: m.Redaction != nil && m.Redaction.Status == report.RedactionFailed}
	if s.Takes == nil {
		s.Takes = []string{}
	}
	if len(takes) == 0 {
		s.Status = WireNone
		if m.Mark == lifecycle.MarkNone && (m.Phase == lifecycle.Queued || m.Active()) {
			s.Status = WirePending
		}
	}
	if slices.Contains(takes, wire.SuiteTake) {
		s.State = wire.StateTaken
	} else if sum := r.a.bundleSummary(run.dir); sum != nil && sum.Wire != nil {
		s.State, s.Detail = sum.Wire.State, run.mask.text(sum.Wire.Detail)
	}
	return s, nil
}

func defaultTake(takes []string) string {
	if slices.Contains(takes, wire.SuiteTake) {
		return wire.SuiteTake
	}
	return takes[len(takes)-1]
}

func (r *Reader) wireRecord(run wireRun, take string) (*wireRecord, error) {
	version, err := run.store.IndexVersion(take)
	if err != nil {
		return nil, run.mask.problem(err)
	}
	rec, err := r.a.wires.read(wireKey{run: run.view.ID, take: take, version: version}, func() (*wireRecord, error) {
		index, err := run.store.Index(take)
		if err != nil {
			return nil, err
		}
		return newWireRecord(index, r.stepTitles(run.view.ID)), nil
	})
	if err != nil {
		return nil, run.mask.problem(err)
	}
	return rec, nil
}

func (r *Reader) stepTitles(id string) map[wireStepKey]string {
	titles := map[wireStepKey]string{}
	_, flows, err := r.Flows(id)
	if err != nil {
		return titles
	}
	for _, f := range flows {
		for _, s := range f.StepList {
			titles[wireStepKey{f.ID, s.ID}] = s.Title
		}
	}
	return titles
}

func (a *api) wireView(w http.ResponseWriter, r *http.Request) error {
	offset, err := intParam(r, "offset", 0)
	if err != nil {
		return err
	}
	limit, err := intParam(r, "limit", 0)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	view, err := a.reader().Wire(r.PathValue("id"), WireQuery{
		Take: q.Get("take"), Flow: q.Get("flow"), Step: q.Get("step"), Scope: q.Get("scope"), Target: q.Get("target"),
		Offset: int(min(offset, 1<<31-1)), Limit: int(min(limit, wirePageMax)),
	})
	if err != nil {
		return err
	}
	writeJSON(a.log(), w, http.StatusOK, view)
	return nil
}

func (a *api) wireCall(w http.ResponseWriter, r *http.Request) error {
	seq, err := strconv.ParseInt(r.PathValue("seq"), 10, 64)
	if err != nil || seq < 0 {
		return badRequest("seq is a non-negative integer")
	}
	view, err := a.reader().WireCall(r.PathValue("id"), r.URL.Query().Get("take"), seq)
	if err != nil {
		return err
	}
	writeJSON(a.log(), w, http.StatusOK, view)
	return nil
}

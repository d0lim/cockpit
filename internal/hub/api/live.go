package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/signal"
)

const (
	SourceLive    = "live"
	SourceBundle  = "bundle"
	HeaderSource  = "X-Cockpit-Source"
	ScriptsDir    = "work/scripts"
	maxSignalRead = 5000
)

type FlowView struct {
	Source string               `json:"source"`
	Flow   contract.FlowSummary `json:"flow"`
}

type SourceView struct {
	Source string `json:"source"`
	File   string `json:"file"`
	Lines  int    `json:"lines"`
	Text   string `json:"text"`
}

type SignalsView struct {
	signal.ReadResponse
	Collectors []CollectorState `json:"collectors"`
}

func (a *api) bundleSummary(dir string) *contract.Summary {
	raw, f := report.ReadConfined(filepath.Join(dir, report.DirReport), report.FileSummary)
	var s contract.Summary
	if f != report.ReadOK || json.Unmarshal(raw, &s) != nil {
		return nil
	}
	return &s
}

func (a *api) flow(w http.ResponseWriter, r *http.Request) error {
	v, err := a.runOf(r)
	if err != nil {
		return err
	}
	dir, err := a.runDir(v.ID)
	if err != nil {
		return err
	}
	id := r.PathValue("flow")
	source, flows := SourceBundle, []contract.FlowSummary(nil)
	if s := a.bundleSummary(dir); s != nil {
		flows = s.Flows
	} else {
		c := report.ReadRunnerFiles(filepath.Join(dir, RunnerDir))
		source, flows = SourceLive, report.FlowSummaries(c.Manifest, c.Result)
		if a.d.Mask != nil {
			m := a.d.Mask(v.ID)
			for i := range flows {
				for j := range flows[i].StepList {
					flows[i].StepList[j].Message = m.Mask(flows[i].StepList[j].Message)
				}
			}
		}
	}
	for _, f := range flows {
		if f.ID == id {
			w.Header().Set(HeaderSource, source)
			writeJSON(a.log(), w, http.StatusOK, FlowView{Source: source, Flow: f})
			return nil
		}
	}
	return notFound("no such flow in this run: " + id)
}

func manifestFiles(m *contract.Manifest) []string {
	var out []string
	for _, f := range m.Flows {
		if f.Source != nil {
			out = append(out, f.Source.File)
		}
		for _, s := range f.Steps {
			if s.Source != nil {
				out = append(out, s.Source.File)
			}
		}
	}
	return out
}

func (a *api) source(w http.ResponseWriter, r *http.Request) error {
	v, err := a.runOf(r)
	if err != nil {
		return err
	}
	dir, err := a.runDir(v.ID)
	if err != nil {
		return err
	}
	file := r.URL.Query().Get("file")
	if !filepath.IsLocal(file) {
		return badRequest("file is a path inside the suite")
	}
	view := SourceView{Source: SourceBundle, File: file}
	var data []byte
	failure := report.ReadMissing
	if a.bundleSummary(dir) != nil {
		data, failure = report.ReadConfined(filepath.Join(dir, report.DirReport, report.BundleSource), file)
	} else {
		view.Source = SourceLive
		c := report.ReadRunnerFiles(filepath.Join(dir, RunnerDir))
		if c.Manifest.Value == nil || !slices.Contains(manifestFiles(c.Manifest.Value), file) {
			return notFound("the manifest does not point to " + file)
		}
		if a.d.Mask == nil {
			return &Problem{Status: http.StatusConflict, Code: CodeNotAvailable, Message: "no run masker: live source is not served"}
		}
		data, failure = report.ReadConfined(filepath.Join(dir, ScriptsDir), file)
		data = []byte(a.d.Mask(v.ID).Mask(string(data)))
	}
	if failure != report.ReadOK {
		return notFound("the source file cannot be read: " + string(failure))
	}
	view.Text = string(data)
	view.Lines = strings.Count(view.Text, "\n") + 1
	w.Header().Set(HeaderSource, view.Source)
	writeJSON(a.log(), w, http.StatusOK, view)
	return nil
}

func intParam(r *http.Request, key string, fallback int64) (int64, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, badRequest(key + " is a non-negative integer")
	}
	return n, nil
}

func (a *api) signals(w http.ResponseWriter, r *http.Request) error {
	v, err := a.runOf(r)
	if err != nil {
		return err
	}
	after, err := intParam(r, "after", 0)
	if err != nil {
		return err
	}
	limit, err := intParam(r, "limit", 0)
	if err != nil {
		return err
	}
	if a.d.Signals == nil {
		return &Problem{Status: http.StatusConflict, Code: CodeNotAvailable, Message: "this hub has no signal store"}
	}
	resp, err := a.d.Signals.Read(v.ID, signal.ReadRequest{After: after, Limit: int(min(limit, maxSignalRead))})
	if errors.Is(err, signal.ErrReadRefused) {
		return &Problem{Status: http.StatusConflict, Code: CodeReadRefused, Message: "signal reads are refused for this run"}
	}
	if err != nil {
		return err
	}
	view := SignalsView{ReadResponse: resp, Collectors: []CollectorState{}}
	if a.d.Collectors != nil {
		if states := a.d.Collectors.Status(v.ID); states != nil {
			view.Collectors = states
		}
	}
	writeJSON(a.log(), w, http.StatusOK, view)
	return nil
}

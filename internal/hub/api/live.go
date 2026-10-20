package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"

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
	view, err := a.reader().Flow(r.PathValue("id"), r.PathValue("flow"))
	if err != nil {
		return err
	}
	w.Header().Set(HeaderSource, view.Source)
	writeJSON(a.log(), w, http.StatusOK, view)
	return nil
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
	view, err := a.reader().Source(r.PathValue("id"), r.URL.Query().Get("file"))
	if err != nil {
		return err
	}
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
	if _, err := a.runOf(r); err != nil {
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
	view, err := a.reader().Signals(r.PathValue("id"), after, int(min(limit, maxSignalRead)))
	if err != nil {
		return err
	}
	writeJSON(a.log(), w, http.StatusOK, view)
	return nil
}

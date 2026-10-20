package screen

import (
	"net/http"
	"slices"

	"github.com/a-h/templ"

	"github.com/d0lim/cockpit/internal/hub/sse"
	"github.com/d0lim/cockpit/internal/hub/store"
)

type region struct {
	id      string
	kinds   []sse.Kind
	signals bool
	render  func(*state) templ.Component
}

type view struct {
	pattern string
	title   string
	tab     string
	run     bool
	served  bool
	check   func(*state) (int, string)
	own     []region
	body    func(*state) templ.Component
}

func (v view) titleOf(st *state) string {
	if st.run == nil {
		return v.title
	}
	return st.run.ID + " · " + v.title
}

var (
	railRegion    = region{id: "rail", kinds: []sse.Kind{sse.KindRuns}, render: rail}
	runHeadRegion = region{id: "run-head", kinds: []sse.Kind{sse.KindRuns}, render: runHead}
)

func (v view) regions() []region {
	out := []region{railRegion}
	if v.run {
		out = append(out, runHeadRegion)
	}
	return append(out, v.own...)
}

const (
	TabOverview = "overview"
	TabReport   = "report"
	TabSignals  = "signals"
	TabEnv      = "env"
	TabPipeline = "pipeline"
)

var signalViews = []string{"trace", "events", "table", "logs"}

var views = []view{
	{
		pattern: "/{$}", title: "Runs",
		own:  []region{{id: "runs", kinds: []sse.Kind{sse.KindRuns}, render: runsRegion}},
		body: runsPage,
	},
	{
		pattern: "/hub", title: "Hub",
		own:  []region{{id: "hub", kinds: []sse.Kind{sse.KindRuns}, render: hubRegion}},
		body: hubPage,
	},
	{
		pattern: "/stacks", title: "Other stacks",
		own:  []region{{id: "stacks", kinds: []sse.Kind{sse.KindRuns}, render: stacksRegion}},
		body: stacksPage,
	},
	{
		pattern: "/runs/{id}", title: "Overview", tab: TabOverview, run: true,
		own:  []region{{id: "run-overview", kinds: []sse.Kind{sse.KindRuns}, render: overviewRegion}},
		body: overviewPage,
	},
	{
		pattern: "/runs/{id}/report", title: "Report", tab: TabReport, run: true, served: true,
		own:  []region{{id: "report", kinds: []sse.Kind{sse.KindRuns}, render: reportRegion}},
		body: reportPage,
	},
	{
		pattern: "/runs/{id}/report/{flow}", title: "Flow", tab: TabReport, run: true, served: true, check: flowExists,
		own:  []region{{id: "flow", kinds: []sse.Kind{sse.KindRuns}, render: flowRegion}},
		body: flowPage,
	},
	{
		pattern: "/runs/{id}/signals/{view}", title: "Signals", tab: TabSignals, run: true, served: true, check: signalViewExists,
		own: []region{
			{id: "signals", signals: true, render: signalsRegion},
			{id: "collectors", kinds: []sse.Kind{sse.KindRuns}, signals: true, render: collectorsRegion},
		},
		body: signalsPage,
	},
	{
		pattern: "/runs/{id}/env", title: "Environment", tab: TabEnv, run: true, served: true,
		own:  []region{{id: "env", kinds: []sse.Kind{sse.KindRuns}, render: envRegion}},
		body: envPage,
	},
	{
		pattern: "/runs/{id}/pipeline", title: "Pipeline", tab: TabPipeline, run: true,
		own:  []region{{id: "pipeline", kinds: []sse.Kind{sse.KindJob}, render: pipelineRegion}},
		body: pipelinePage,
	},
}

func (st *state) pickRun(id string) (int, string) {
	if !store.IsRunID(id) {
		return http.StatusBadRequest, "That is not a run id: run-<8-digit date>-<6-digit time>-<7-character commit>."
	}
	for i := range st.list.Runs {
		if st.list.Runs[i].ID == id {
			st.run = &st.list.Runs[i]
			return http.StatusOK, ""
		}
	}
	return http.StatusNotFound, "This hub has no run " + id + "."
}

func flowExists(st *state) (int, string) {
	if _, err := st.s.d.Reader.Flow(st.run.ID, st.values["flow"]); err != nil {
		return http.StatusNotFound, "This run has no flow " + st.values["flow"] + "."
	}
	return http.StatusOK, ""
}

func signalViewExists(st *state) (int, string) {
	if !slices.Contains(signalViews, st.values["view"]) {
		return http.StatusNotFound, "Signals are shown as trace, events, table or logs."
	}
	return http.StatusOK, ""
}

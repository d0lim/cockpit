package screen

import (
	"net/http"

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
	run     bool
	own     []region
	body    func(*state) templ.Component
}

var railRegion = region{id: "rail", kinds: []sse.Kind{sse.KindRuns}, render: rail}

func (v view) regions() []region {
	out := []region{railRegion}
	return append(out, v.own...)
}

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

package screen

import (
	"errors"
	"net/http"
	"slices"

	"github.com/a-h/templ"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
)

const maxForm = 64 << 10

func (s *Screen) act(w http.ResponseWriter, r *http.Request) {
	id, op := r.PathValue("id"), r.PathValue("op")
	r.Body = http.MaxBytesReader(w, r.Body, maxForm)
	if err := r.ParseForm(); err != nil {
		s.answer(w, r, http.StatusBadRequest, problemResult(&api.Problem{Code: api.CodeBadRequest, Message: "the form cannot be read"}))
		return
	}
	actor := scheduler.Actor{By: api.UISubmitter, Force: r.PostForm.Get("force") == "1"}
	switch {
	case op == api.OpExec:
		view, err := s.d.Reader.Exec(id, r.PostForm.Get("flow"), actor)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.answer(w, r, http.StatusOK, execResult(r.PostForm.Get("flow"), view))
	case slices.Contains(api.Ops, op):
		reply, status, err := s.d.Reader.Operate(id, op, actor)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.answer(w, r, status, opResult(op, reply.Run))
	default:
		s.answer(w, r, http.StatusNotFound, problemResult(&api.Problem{Code: api.CodeNotFound, Message: "there is no action " + op}))
	}
}

func (s *Screen) fail(w http.ResponseWriter, r *http.Request, err error) {
	var p *api.Problem
	if !errors.As(err, &p) {
		s.d.Log.Warn("screen action failed", "path", r.URL.Path, "error", err)
		p = &api.Problem{Status: http.StatusInternalServerError, Code: api.CodeInternal, Message: "the hub could not do this"}
	}
	s.answer(w, r, p.Status, problemResult(p))
}

func (s *Screen) answer(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	headers(w)
	w.WriteHeader(status)
	if err := c.Render(r.Context(), w); err != nil {
		s.d.Log.Warn("screen answer failed", "path", r.URL.Path, "error", err)
	}
}

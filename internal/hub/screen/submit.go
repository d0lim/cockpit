package screen

import (
	"errors"
	"net/http"
	"strings"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
)

const defaultCommit = "HEAD"

func (s *Screen) submit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxForm)
	if err := r.ParseForm(); err != nil {
		s.answer(w, r, http.StatusBadRequest, problemResult(&api.Problem{Code: api.CodeBadRequest, Message: "the form cannot be read"}))
		return
	}
	in := scheduler.SubmitInput{
		Worktree:  strings.TrimSpace(r.PostForm.Get("worktree")),
		Commit:    orElse(strings.TrimSpace(r.PostForm.Get("commit")), defaultCommit),
		Suite:     strings.TrimSpace(r.PostForm.Get("suite")),
		Submitter: api.UISubmitter,
	}
	if in.Worktree == "" {
		s.answer(w, r, http.StatusBadRequest, problemResult(&api.Problem{Code: api.CodeBadRequest, Message: "pick a repository"}))
		return
	}
	got, err := s.d.Reader.Submit(r.Context(), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.answer(w, r, http.StatusAccepted, submittedResult(got))
}

func (s *Screen) suites(w http.ResponseWriter, r *http.Request) {
	view, err := s.d.Reader.Suites(r.Context(), r.URL.Query().Get("worktree"), r.URL.Query().Get("commit"))
	if err != nil {
		var msg string
		var p *api.Problem
		if errors.As(err, &p) {
			msg = p.Message
		}
		s.answer(w, r, http.StatusOK, suiteOptions(api.SuitesView{}, orElse(msg, "the suites cannot be read")))
		return
	}
	s.answer(w, r, http.StatusOK, suiteOptions(view, ""))
}

func submittedText(got api.Submitted) string {
	switch {
	case got.Merged:
		return "The same commit and suite were already queued, so the request joined " + got.ID + "."
	case got.Position > 0:
		return "Submitted " + got.ID + ". It waits in the queue at position " + itoa(got.Position) + "."
	}
	return "Submitted " + got.ID + "."
}

package api

import (
	"context"
	"slices"

	"github.com/d0lim/cockpit/internal/hub/scheduler"
)

func (r *Reader) Repositories() []string { return slices.Clone(r.a.d.Repos) }

func (r *Reader) Suites(ctx context.Context, worktree string) (SuitesView, error) {
	view := SuitesView{Worktree: worktree, Suites: []SuiteName{}}
	if r.a.d.Suites == nil || worktree == "" {
		return view, nil
	}
	names, err := r.a.d.Suites(ctx, worktree)
	if err != nil {
		return SuitesView{}, badRequest(err.Error())
	}
	for _, n := range names {
		view.Suites = append(view.Suites, SuiteName{Name: n})
	}
	return view, nil
}

func (r *Reader) Submit(ctx context.Context, in scheduler.SubmitInput) (Submitted, error) {
	if in.Suite != "" && !suitePattern.MatchString(in.Suite) {
		return Submitted{}, badRequest("the suite name must be lowercase letters, digits and -, up to 63 characters")
	}
	res := r.a.d.Scheduler.Submit(ctx, in)
	r.a.changed()
	if res.Fail != nil {
		return Submitted{}, fromFailure(res.Fail)
	}
	return Submitted{ID: res.ID, Phase: res.Phase, Position: res.Position, Merged: res.Merged}, nil
}

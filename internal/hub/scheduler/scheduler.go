package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
)

type Actor struct {
	By    string
	Force bool
}

type SubmitInput struct {
	Worktree  string
	Commit    string
	Suite     string
	Submitter string
	New       bool
}

type Accepted struct {
	SHA        string
	HostAPI    int
	Repository string
	Suite      string
}

type RejectKind string

const (
	KindCommit      RejectKind = "commit"
	KindWorktree    RejectKind = "worktree"
	KindDirty       RejectKind = "dirty"
	KindDeclaration RejectKind = "declaration"
)

var rejectCodes = map[RejectKind]ticker.Code{
	KindCommit:      ticker.CodeRejectedCommit,
	KindWorktree:    ticker.CodeRejectedWorktree,
	KindDirty:       ticker.CodeRejectedDirty,
	KindDeclaration: ticker.CodeRejectedNoHarness,
}

type Rejection struct {
	Kind    RejectKind
	Message string
	Files   []string
}

func (r *Rejection) Error() string { return r.Message }

type ExecResponse struct {
	Result any
	Fail   *ticker.Failure
}

type Execer interface {
	Denied(m store.Meta, busy bool, flow string) *ticker.Failure
	Run(ctx context.Context, m store.Meta, flow string) ExecResponse
}

type Deps struct {
	ticker.Deps
	Accept func(ctx context.Context, in SubmitInput) (Accepted, error)
	Exec   Execer
}

type Scheduler struct {
	d        Deps
	core     *ticker.Core
	submitMu sync.Mutex
	execs    sync.Map
}

func New(d Deps) *Scheduler {
	return &Scheduler{d: d, core: ticker.New(d.Deps)}
}

type SubmitResult struct {
	ID       string
	Phase    lifecycle.Phase
	Position int
	Merged   bool
	Fail     *ticker.Failure
}

func failed(status int, code ticker.Code, reason string) SubmitResult {
	return SubmitResult{Fail: &ticker.Failure{Status: status, Code: code, Reason: reason}}
}

func (s *Scheduler) Submit(ctx context.Context, in SubmitInput) SubmitResult {
	accepted, err := s.d.Accept(ctx, in)
	var rejection *Rejection
	if errors.As(err, &rejection) {
		r := failed(400, rejectCodes[rejection.Kind], rejection.Message)
		r.Fail.Files = rejection.Files
		return r
	}
	if err != nil {
		return failed(500, ticker.CodeOpFailed, err.Error())
	}
	if !lifecycle.Serves(s.d.Served, accepted.HostAPI) {
		return failed(400, ticker.CodeRejectedHostAPI, lifecycle.HostAPINotice(accepted.HostAPI, s.d.Served, s.d.CockpitVersion))
	}
	s.submitMu.Lock()
	defer s.submitMu.Unlock()
	st, err := s.d.Store.CurrentState()
	if err != nil {
		return failed(500, ticker.CodeOpFailed, err.Error())
	}
	if accepted.Suite != "" {
		in.Suite = accepted.Suite
	}
	if !in.New {
		for i, id := range st.Queue {
			if m := s.find(id); m != nil && m.Phase == lifecycle.Queued && m.SHA == accepted.SHA && m.Suite == in.Suite {
				return SubmitResult{ID: id, Phase: m.Phase, Position: i + 1, Merged: true}
			}
		}
	}
	if len(st.Queue) >= s.d.Limits.QueueMax {
		return failed(409, ticker.CodeQueueFull, fmt.Sprintf("the queue is full (cap %d)", s.d.Limits.QueueMax))
	}
	list, err := s.d.Store.ListRuns()
	if err != nil {
		return failed(500, ticker.CodeOpFailed, err.Error())
	}
	existing := make([]string, 0, len(list.Runs))
	for _, m := range list.Runs {
		existing = append(existing, m.ID)
	}
	now := s.d.Now()
	created, err := lifecycle.Transition(nil, lifecycle.Submit, lifecycle.Detail{})
	if err != nil {
		return failed(500, ticker.CodeOpFailed, err.Error())
	}
	m := store.Meta{
		State:          created,
		ID:             store.NewRunID(now, accepted.SHA, existing),
		SHA:            accepted.SHA,
		Repository:     accepted.Repository,
		Suite:          in.Suite,
		HostAPI:        accepted.HostAPI,
		CockpitVersion: s.d.CockpitVersion,
		Submitter:      in.Submitter,
		CreatedAt:      store.Timestamp(now),
	}
	if err := s.d.Store.WriteMeta(m); err != nil {
		return failed(500, ticker.CodeOpFailed, err.Error())
	}
	next, err := s.d.Store.UpdateState(func(st store.HubState) store.HubState {
		st.Queue = append(st.Queue, m.ID)
		return st
	})
	if err != nil {
		return failed(500, ticker.CodeOpFailed, err.Error())
	}
	s.core.Kick()
	return SubmitResult{ID: m.ID, Phase: m.Phase, Position: len(next.Queue), Merged: false}
}

func (s *Scheduler) find(id string) *store.Meta {
	m, err := s.d.Store.ReadMeta(id)
	if err != nil {
		return nil
	}
	return m
}

func notFound(id string) ticker.Result {
	return ticker.Failed(404, ticker.CodeNotFound, "no such run: "+id, nil)
}

func ownerDenied(m store.Meta, a Actor, verb string) *ticker.Result {
	if a.Force || m.Submitter == a.By {
		return nil
	}
	r := ticker.Failed(409, ticker.CodeConflictOwner, fmt.Sprintf("%s submitted this run: pass force to %s another person's run", m.Submitter, verb), &m)
	return &r
}

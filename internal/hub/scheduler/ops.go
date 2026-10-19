package scheduler

import (
	"fmt"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/pool"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
)

func (s *Scheduler) setKeep(id string, keep bool, a Actor) ticker.Result {
	m := s.find(id)
	if m == nil {
		return notFound(id)
	}
	if !keep {
		if denied := ownerDenied(*m, a, "unkeep"); denied != nil {
			return *denied
		}
	}
	if pool.KeepFrozen(*m) {
		return ticker.Conflict(lifecycle.Label(m.State)+" state does not change keep", m)
	}
	run := *m
	if m.Keep != keep {
		patched, err := s.d.Store.PatchMeta(id, func(x *store.Meta) { x.Keep = keep })
		if err != nil {
			return ticker.Failed(500, ticker.CodeOpFailed, err.Error(), m)
		}
		run = patched
	}
	s.core.Kick()
	return ticker.Succeeded(run)
}

func (s *Scheduler) Keep(id string, a Actor) ticker.Result { return s.setKeep(id, true, a) }

func (s *Scheduler) Unkeep(id string, a Actor) ticker.Result { return s.setKeep(id, false, a) }

func (s *Scheduler) guarded(id string, a Actor, verb string, act func() ticker.Result) ticker.Result {
	m := s.find(id)
	if m == nil {
		return notFound(id)
	}
	if denied := ownerDenied(*m, a, verb); denied != nil {
		return *denied
	}
	return act()
}

func (s *Scheduler) Stop(id string, a Actor) ticker.Result {
	return s.guarded(id, a, "stop", func() ticker.Result { return s.core.StopRun(id, lifecycle.StopPerson) })
}

func (s *Scheduler) Destroy(id string, a Actor) ticker.Result {
	return s.guarded(id, a, "destroy", func() ticker.Result { return s.core.DestroyRun(id, lifecycle.DestroyPerson) })
}

func (s *Scheduler) unserved(m store.Meta) *ticker.Result {
	if lifecycle.Serves(s.d.Served, m.HostAPI) {
		return nil
	}
	r := ticker.Conflict(lifecycle.HostAPINotice(m.HostAPI, s.d.Served, s.d.CockpitVersion), &m)
	return &r
}

func (s *Scheduler) Resume(id string, _ Actor) ticker.Result {
	m := s.find(id)
	if m == nil {
		return notFound(id)
	}
	if refused := s.unserved(*m); refused != nil {
		return *refused
	}
	if _, err := lifecycle.Transition(&m.State, lifecycle.Resume, lifecycle.Detail{}); err != nil {
		return ticker.Conflict(err.Error(), m)
	}
	if err := s.core.RefreshProjects(s.core.Context()); err != nil {
		return ticker.Failed(500, ticker.CodeOpFailed, err.Error(), m)
	}
	list, err := s.d.Store.ListRuns()
	if err != nil {
		return ticker.Failed(500, ticker.CodeOpFailed, err.Error(), m)
	}
	view := s.core.View(list.Runs)
	if !pool.CanResume(view, s.core.ActiveShare(view)) {
		r := ticker.Failed(409, ticker.CodeConflictCap, fmt.Sprintf("the running environments are at the cap %d: stop one of the candidates first", s.d.Limits.UpCap), m)
		r.Fail.Candidates = pool.ResumeCandidates(view.Runs)
		return r
	}
	current, err := s.d.Store.ApplyEvent(id, lifecycle.Resume, lifecycle.Detail{}, func(x *store.Meta) { x.StopReason = lifecycle.StopNone })
	if err != nil {
		return ticker.Conflict(err.Error(), &current)
	}
	return s.core.RunOp(id, func() ticker.Result { return s.d.Env.Resume(s.core.Context(), id) }, true)
}

func (s *Scheduler) Abort(id string, _ Actor) ticker.Result {
	m := s.find(id)
	if m == nil {
		return notFound(id)
	}
	if a := s.core.ActiveRun(); a == nil || a.ID != id {
		return ticker.Conflict(lifecycle.Label(m.State)+" state is not aborted: only the running run is aborted", m)
	}
	if _, err := lifecycle.Transition(&m.State, lifecycle.Abort, lifecycle.Detail{}); err != nil {
		return ticker.Conflict(err.Error(), m)
	}
	s.core.Abort(id)
	return ticker.Succeeded(*m)
}

func (s *Scheduler) Exec(id, flow string, _ Actor) ExecResponse {
	m := s.find(id)
	if m == nil {
		return ExecResponse{Fail: notFound(id).Fail}
	}
	if refused := s.unserved(*m); refused != nil {
		return ExecResponse{Fail: refused.Fail}
	}
	if denied := s.d.Exec.Denied(*m, s.core.Busy(id), flow); denied != nil {
		return ExecResponse{Fail: denied}
	}
	var resp ExecResponse
	s.core.RunOp(id, func() ticker.Result {
		resp = s.d.Exec.Run(s.core.Context(), *m, flow)
		return ticker.Result{}
	}, false)
	return resp
}

type RunView struct {
	store.Meta
	Position int `json:"position,omitempty"`
}

type Count struct {
	Count int `json:"count"`
	Cap   int `json:"cap"`
}

type Size struct {
	Size int `json:"size"`
	Max  int `json:"max"`
}

type HubList struct {
	Runs       []RunView          `json:"runs"`
	Orphans    []reconcile.Orphan `json:"orphans"`
	Pool       Size               `json:"pool"`
	Up         Count              `json:"up"`
	DockerDown bool               `json:"dockerDown"`
	TickError  string             `json:"tickError"`
}

func positioned(m store.Meta, queue []string) RunView {
	if m.Phase == lifecycle.Queued {
		for i, id := range queue {
			if id == m.ID {
				return RunView{Meta: m, Position: i + 1}
			}
		}
	}
	return RunView{Meta: m}
}

func (s *Scheduler) List() (HubList, error) {
	list, err := s.d.Store.ListRuns()
	if err != nil {
		return HubList{}, err
	}
	st, err := s.d.Store.CurrentState()
	if err != nil {
		return HubList{}, err
	}
	view := s.core.View(list.Runs)
	out := HubList{Orphans: s.core.Orphans(), DockerDown: s.core.DockerDown(), TickError: s.core.TickError()}
	for _, m := range list.Runs {
		out.Runs = append(out.Runs, positioned(m, st.Queue))
		if pool.InPool(m) {
			out.Pool.Size++
		}
	}
	out.Pool.Max = s.d.Limits.PoolMax
	out.Up = Count{Count: pool.UpCount(view, s.core.HasActive(view)), Cap: s.d.Limits.UpCap}
	return out, nil
}

func (s *Scheduler) Get(id string) *RunView {
	m := s.find(id)
	if m == nil {
		return nil
	}
	st, _ := s.d.Store.CurrentState()
	v := positioned(*m, st.Queue)
	return &v
}

func (s *Scheduler) WaitReasonOf(id string) lifecycle.WaitReason {
	if m := s.find(id); m != nil && m.Phase == lifecycle.Queued {
		return m.WaitReason
	}
	return lifecycle.WaitNone
}

func (s *Scheduler) CandidatesForResume() []pool.Candidate {
	list, err := s.d.Store.ListRuns()
	if err != nil {
		return nil
	}
	return pool.ResumeCandidates(list.Runs)
}

func (s *Scheduler) AllowedEvents(m store.Meta) []lifecycle.Event {
	return lifecycle.AllowedEvents(&m.State, lifecycle.Serves(s.d.Served, m.HostAPI))
}

func (s *Scheduler) Tick() { s.core.Tick() }

func (s *Scheduler) Reconcile() (skipped bool) { return s.core.Reconcile() }

func (s *Scheduler) Idle() { s.core.Idle() }

func (s *Scheduler) Dispose() { s.core.Dispose() }

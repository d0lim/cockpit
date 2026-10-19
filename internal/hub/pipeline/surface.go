package pipeline

import (
	"context"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/signal"
)

type collectorStatus struct{ h *Hub }

func (c collectorStatus) Status(runID string) []api.CollectorState {
	if c.h.ops.Collect == nil {
		return nil
	}
	statuses := c.h.ops.Collect.Status(runID)
	out := make([]api.CollectorState, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, api.CollectorState{ID: s.ID, State: s.State, LastOKAt: s.LastSuccess, Failures: s.Failures, Reason: s.Reason})
	}
	return out
}

func (h *Hub) Collectors() api.Collectors { return collectorStatus{h} }

func (h *Hub) Streams() *signal.Registry { return h.ops.Streams }

func (h *Hub) Mask(runID string) *mask.Masker { return h.ops.MaskerFor(runID) }

func (h *Hub) Services(m store.Meta) []api.ServicePort {
	r, err := h.ops.Open(m.ID)
	if err != nil || r.Decl == nil || m.Slot == nil {
		if err == nil {
			r.Close()
		}
		return []api.ServicePort{}
	}
	defer r.Close()
	base := h.base(*m.Slot)
	out := make([]api.ServicePort, 0, len(r.Decl.Ports))
	for _, p := range r.Decl.Ports {
		out = append(out, api.ServicePort{Name: string(p.Name), Service: string(p.Service), Port: p.Port, HostPort: compose.HostPort(base, p), Label: p.Display, Open: p.Open})
	}
	return out
}

func (h *Hub) Suites(ctx context.Context, worktree string) ([]string, error) {
	if _, err := h.o.Config.Allow(ctx, compose.CommonDirOf, worktree); err != nil {
		return nil, err
	}
	top, err := compose.Git{Dir: worktree}.TopLevel(ctx)
	if err != nil {
		return nil, err
	}
	git := compose.Git{Dir: top}
	sha, err := git.ResolveCommit(ctx, "HEAD")
	if err != nil {
		return nil, err
	}
	loaded, err := compose.ReadDeclaration(ctx, git, sha)
	if err != nil {
		return nil, err
	}
	return compose.Suites(loaded.Declaration, loaded.Tree), nil
}

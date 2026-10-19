package pipeline

import (
	"context"
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/lease"
	"github.com/d0lim/cockpit/internal/hub/store"
)

const canaryName = "seal-canary"

func yamlUnmarshal(data []byte, v any) error { return yaml.Unmarshal(data, v) }

func (h *Hub) declaration(ctx context.Context, m store.Meta) (compose.Loaded, error) {
	if m.Repository == "" {
		return compose.Loaded{}, fmt.Errorf("run %s records no repository", m.ID)
	}
	return compose.ReadDeclaration(ctx, compose.Git{Dir: m.Repository}, m.SHA)
}

func (h *Hub) runLayout(loaded compose.Loaded) lease.Layout {
	l := h.layout
	l.Offsets = map[string]int{canaryName: compose.CanaryOffset}
	for _, p := range loaded.Declaration.Ports {
		l.Offsets[string(p.Name)] = p.Offset
	}
	return l
}

func (h *Hub) Lease(leases store.Leases, id string) (int, error) {
	m, err := h.store.MustRead(id)
	if err != nil {
		return 0, err
	}
	loaded, err := h.declaration(context.Background(), m)
	if err != nil {
		return 0, err
	}
	_, slot, _, err := lease.Lease(leases, id, h.o.PortProbe, h.runLayout(loaded))
	return slot, err
}

func (h *Hub) base(slot int) int {
	return h.layout.Base + h.layout.Stride*slot
}

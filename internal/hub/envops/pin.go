package envops

import (
	"context"
	"fmt"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/store"
)

type PinOutcome string

const (
	PinMatched    PinOutcome = "matched"
	PinRetagged   PinOutcome = "retagged"
	PinReimported PinOutcome = "reimported"
)

func imageID(ctx context.Context, d compose.Docker, ref string) (string, bool) {
	res, err := d.Run(ctx, compose.Command{Args: []string{"image", "inspect", "--format", "{{.Id}}", ref}})
	id := strings.TrimSpace(string(res.Stdout))
	return id, err == nil && res.Code == 0 && strings.HasPrefix(id, "sha256:")
}

func (o *Ops) EnsurePinned(ctx context.Context, d compose.Docker, pin *contract.RunnerImage) (contract.RunnerImage, PinOutcome, error) {
	if pin == nil || pin.Tag == "" || pin.ID == "" {
		return contract.RunnerImage{}, "", fmt.Errorf("the run has no pinned runner image")
	}
	if current, ok := imageID(ctx, d, pin.Tag); ok && current == pin.ID {
		return *pin, PinMatched, nil
	}
	if _, ok := imageID(ctx, d, pin.ID); ok {
		res, err := d.Run(ctx, compose.Command{Args: []string{"tag", pin.ID, pin.Tag}})
		if err != nil || res.Code != 0 {
			return contract.RunnerImage{}, "", fmt.Errorf("cannot point %s back at the pinned image %s", pin.Tag, pin.ID)
		}
		return *pin, PinRetagged, nil
	}
	img, err := o.Ensure(ctx)
	if err != nil {
		return contract.RunnerImage{}, "", fmt.Errorf("the pinned runner image %s is gone and cannot be imported again: %w", pin.ID, err)
	}
	if img.Tag != pin.Tag {
		return contract.RunnerImage{}, "", fmt.Errorf("the pinned runner %s is gone and this hub embeds %s instead", pin.Tag, img.Tag)
	}
	return contract.RunnerImage{Tag: img.Tag, ID: img.ID}, PinReimported, nil
}

func (o *Ops) VerifyPin(ctx context.Context, r *Run) error {
	pin, outcome, err := o.EnsurePinned(ctx, r.Docker, r.Meta.RunnerImage)
	if err != nil {
		return err
	}
	if outcome == PinReimported {
		m, err := r.Store.PatchMeta(r.Meta.ID, func(m *store.Meta) { m.RunnerImage = &pin })
		if err != nil {
			return err
		}
		r.Meta = m
	}
	return nil
}

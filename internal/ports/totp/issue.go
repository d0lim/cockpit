package totp

import (
	"context"

	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/clock"
)

type Steps interface {
	LastStep(loginID string) (int64, bool)
	CommitStep(loginID string, step int64) error
}

type Issuer struct {
	Seeds func(loginID string) (string, bool)
	Clock clock.Clock
	Sleep func(ctx context.Context, ms int64) error
	Steps Steps
	Mask  ports.Mask
}

func (i Issuer) Issue(ctx context.Context, loginID string) (string, error) {
	seed, ok := i.Seeds(loginID)
	if !ok {
		return "", ports.BadArgs(i.Mask, loginID+" is not an account in the seed key table")
	}
	if seed == "" {
		return "", ports.Unavailable(i.Mask, "two-factor seed for "+loginID+" is empty")
	}
	last, has := i.Steps.LastStep(loginID)
	step, wait := ChooseStep(i.Clock.Millis(), last, has)
	if wait > 0 {
		if err := i.Sleep(ctx, wait); err != nil {
			return "", ports.Unavailable(i.Mask, "wait for the next two-factor window for "+loginID+" was cancelled")
		}
	}
	if err := i.Steps.CommitStep(loginID, step); err != nil {
		return "", ports.Unavailable(i.Mask, "failed to record the two-factor step for "+loginID)
	}
	code, err := At(seed, step)
	if err != nil {
		return "", ports.Unavailable(i.Mask, "failed to read the two-factor seed for "+loginID)
	}
	return code, nil
}

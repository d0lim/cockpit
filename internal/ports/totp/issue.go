package totp

import (
	"context"

	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/clock"
)

type Steps interface {
	ReserveStep(loginID string, choose func(last int64, has bool) int64) (int64, error)
}

type Issuer struct {
	Seeds func(loginID string) (string, bool)
	Clock clock.Clock
	Sleep func(ctx context.Context, ms int64) error
	Steps Steps
	Mask  ports.Mask

	StepMillis int64
}

func (i Issuer) Issue(ctx context.Context, loginID string) (string, error) {
	seed, ok := i.Seeds(loginID)
	if !ok {
		return "", ports.BadArgs(i.Mask, loginID+" is not an account in the seed key table")
	}
	if seed == "" {
		return "", ports.Unavailable(i.Mask, "two-factor seed for "+loginID+" is empty")
	}
	var wait int64
	step, err := i.Steps.ReserveStep(loginID, func(last int64, has bool) int64 {
		var step int64
		step, wait = ChooseStep(i.Clock.Millis(), last, has, i.StepMillis)
		return step
	})
	if err != nil {
		return "", ports.Unavailable(i.Mask, "failed to record the two-factor step for "+loginID)
	}
	if wait > 0 {
		if err := i.Sleep(ctx, wait); err != nil {
			return "", ports.Unavailable(i.Mask, "wait for the next two-factor window for "+loginID+" was cancelled")
		}
	}
	code, err := At(seed, step)
	if err != nil {
		return "", ports.Unavailable(i.Mask, "failed to read the two-factor seed for "+loginID)
	}
	return code, nil
}

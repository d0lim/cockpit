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
		return "", ports.BadArgs(i.Mask, loginID+" 는 시드 키 표에 없는 계정이다")
	}
	if seed == "" {
		return "", ports.Unavailable(i.Mask, loginID+" 의 2팩터 시드가 비어 있다")
	}
	last, has := i.Steps.LastStep(loginID)
	step, wait := ChooseStep(i.Clock.Millis(), last, has)
	if wait > 0 {
		if err := i.Sleep(ctx, wait); err != nil {
			return "", ports.Unavailable(i.Mask, loginID+" 의 2팩터 창 대기가 취소됐다")
		}
	}
	if err := i.Steps.CommitStep(loginID, step); err != nil {
		return "", ports.Unavailable(i.Mask, loginID+" 의 2팩터 스텝을 적지 못했다")
	}
	code, err := At(seed, step)
	if err != nil {
		return "", ports.Unavailable(i.Mask, loginID+" 의 2팩터 시드를 읽지 못했다")
	}
	return code, nil
}

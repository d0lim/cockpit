package exec

import (
	"context"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/hub/runfiles"
	"github.com/d0lim/cockpit/internal/wire"
)

const wireArmFailed = "wire record: the recorder could not be armed, so this exec is not recorded"

type execRecording struct {
	wire *envops.RunWire
	take string
	note string
}

func armWire(ctx context.Context, ops *envops.Ops, r *envops.Run, take string) execRecording {
	if !r.Meta.Wire {
		return execRecording{}
	}
	ctx = context.WithoutCancel(ctx)
	w := ops.RunWire(r.Meta.ID, r.Layout.Root, r.Decl)
	suite := w.Take(ctx, wire.SuiteTake, suiteResult(r))
	if suite.State != wire.StateTaken && suite.Detail != wire.DetailRestarted {
		return execRecording{note: "wire record: the suite record is not stored (" + string(suite.State) + ": " + suite.Detail + "), so the recorder keeps it and this exec is not recorded"}
	}
	if err := w.Arm(ctx, take); err != nil {
		return execRecording{note: wireArmFailed}
	}
	return execRecording{wire: w, take: take}
}

func suiteResult(r *envops.Run) *contract.Result {
	result, err := runfiles.Result(r.Root)
	if err != nil {
		return nil
	}
	return result
}

func (e execRecording) finish(ctx context.Context, result *contract.Result) []string {
	if e.note != "" {
		return []string{e.note}
	}
	if e.wire == nil {
		return nil
	}
	status := e.wire.Take(context.WithoutCancel(ctx), e.take, result)
	if status.State == wire.StateTaken {
		return nil
	}
	return []string{"wire record: take " + e.take + " " + string(status.State) + ": " + status.Detail}
}

func resultOf(result *contract.ExecResult, err error) *contract.Result {
	if err != nil || result == nil {
		return nil
	}
	return &result.Result
}

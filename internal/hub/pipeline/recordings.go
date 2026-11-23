package pipeline

import (
	"context"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/signal"
	"github.com/d0lim/cockpit/internal/wire"
)

type Recordings struct {
	Stream *signal.Stream
	Exited time.Time
	Now    time.Time
	Wire   *envops.RunWire
}

func AssembleReport(ctx context.Context, bundle report.Bundle, rec Recordings) (report.Outcome, error) {
	result := bundle.Inputs.Result.Value
	spans := reportSpans(rec, bundle.Meta, result, bundle.Mask)
	wireStatus, wireUnread := takeSuiteWire(ctx, rec.Wire, result)
	bundle.Unread = append(append(bundle.Unread, spans...), wireUnread...)
	bundle.Summary.Wire = wireStatus
	if rec.Wire != nil {
		bundle.Keep = wire.StagingDir
	}
	out, err := report.Assemble(bundle)
	if rec.Wire != nil && out.Dir != "" {
		rec.Wire.Attach()
	}
	return out, err
}

func takeSuiteWire(ctx context.Context, w *envops.RunWire, result *contract.Result) (*contract.WireStatus, []report.Unread) {
	if w == nil {
		return nil, nil
	}
	status := w.TakeSuite(ctx, result)
	if status.State == contract.WireStateTaken {
		return &status, nil
	}
	return &status, []report.Unread{{Source: stageWire, Detail: string(status.State) + ": " + status.Detail, Count: 1}}
}

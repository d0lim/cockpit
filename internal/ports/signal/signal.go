package signalport

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/signal"
)

const DefaultMaxBytes = 64 << 10

type Emitter interface {
	Append(sig contract.Signal) (int64, error)
}

type Port struct {
	Collector string
	Store     Emitter
	Mask      ports.Mask
	MaxBytes  int
}

type input struct {
	TS       string              `json:"ts"`
	TSSource contract.TimeSource `json:"tsSource"`
	ID       string              `json:"id"`
	Rev      int                 `json:"rev"`
	Attrs    contract.Attrs      `json:"attrs"`
	Name     string              `json:"name"`
	Value    *float64            `json:"value"`
	Body     string              `json:"body"`
	TraceID  string              `json:"traceId"`
	ParentID string              `json:"parentId"`
	Start    string              `json:"start"`
	End      string              `json:"end"`
	Status   contract.SpanStatus `json:"status"`
	Links    []contract.SpanLink `json:"links"`
}

func (p *Port) mask() ports.Mask {
	if p.Mask == nil {
		return ports.Identity
	}
	return p.Mask
}

func (p *Port) Call(_ context.Context, method string, args []json.RawMessage) (any, error) {
	mask := p.mask()
	limit := p.MaxBytes
	if limit <= 0 {
		limit = DefaultMaxBytes
	}
	if len(args) > 0 && len(args[0]) > limit {
		return nil, ports.Limit(mask, method+": the signal is larger than the limit")
	}
	var in input
	if err := ports.Arg(args, 0, &in, mask, method+" input"); err != nil {
		return nil, err
	}
	if in.TSSource == contract.TimeSourceRunner {
		return nil, ports.Denied(mask, method+": the runner time source is reserved for spans the hub builds from runner results")
	}
	sig := contract.Signal{TS: in.TS, TSSource: in.TSSource, Collector: p.Collector, ID: in.ID, Rev: in.Rev, Attrs: in.Attrs}
	switch method {
	case "metric":
		if in.Value == nil || in.Name == "" {
			return nil, ports.BadArgs(mask, "metric needs a name and a numeric value")
		}
		sig.Kind, sig.Metric = contract.SignalKindMetric, &contract.MetricBody{Name: in.Name, Value: *in.Value}
	case "event":
		sig.Kind, sig.Event = contract.SignalKindEvent, &contract.EventBody{Body: in.Body}
	case "span":
		if in.TraceID == "" || in.Name == "" || in.Start == "" {
			return nil, ports.BadArgs(mask, "span needs a traceId, a name and a start")
		}
		sig.Kind = contract.SignalKindSpan
		sig.Span = &contract.SpanBody{TraceID: in.TraceID, ParentID: in.ParentID, Name: in.Name, Start: in.Start, End: in.End, Status: in.Status, Links: in.Links}
	default:
		return nil, ports.UnknownMethod(mask, ports.PortSignal, method)
	}
	if _, err := p.Store.Append(sig); err != nil {
		if errors.Is(err, signal.ErrNoMasker) {
			return nil, ports.Unavailable(mask, "the signal store has no masker for this run")
		}
		return nil, ports.BadArgs(mask, method+": "+err.Error())
	}
	return nil, nil
}

package otlp

import (
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/d0lim/cockpit/internal/contract"
)

func spanSignals(td *tracev1.TracesData) ([]Signal, error) {
	n := 0
	for _, rs := range td.GetResourceSpans() {
		for _, ss := range rs.GetScopeSpans() {
			n += len(ss.GetSpans())
		}
	}
	if n > MaxSignals {
		return nil, errTooMany
	}
	out := make([]Signal, 0, n)
	for _, rs := range td.GetResourceSpans() {
		service := serviceOf(rs.GetResource())
		for _, ss := range rs.GetScopeSpans() {
			for _, s := range ss.GetSpans() {
				out = append(out, spanSignal(s, service))
			}
		}
	}
	return out, nil
}

func spanSignal(s *tracev1.Span, service string) Signal {
	status := contract.SpanStatusOK
	if s.GetStatus().GetCode() == tracev1.Status_STATUS_CODE_ERROR {
		status = contract.SpanStatusError
	}
	var links []contract.SpanLink
	for _, l := range s.GetLinks() {
		links = append(links, contract.SpanLink{TraceID: w3c(l.GetTraceId()), SpanID: w3c(l.GetSpanId())})
	}
	start := stamp(s.GetStartTimeUnixNano())
	return Signal{
		Kind:     contract.SignalKindSpan,
		ID:       w3c(s.GetSpanId()),
		TS:       start,
		TSSource: contract.TimeSourceContainer,
		Attrs:    attrsOf(s.GetAttributes(), service),
		Span: &contract.SpanBody{
			TraceID:  w3c(s.GetTraceId()),
			ParentID: w3c(s.GetParentSpanId()),
			Name:     s.GetName(),
			Start:    start,
			End:      stamp(s.GetEndTimeUnixNano()),
			Status:   status,
			Links:    links,
		},
	}
}

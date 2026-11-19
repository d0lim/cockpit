package envops

import (
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/signal"
	"github.com/d0lim/cockpit/internal/wire"
)

func (w *RunWire) spans(result *contract.Result) []wire.Span {
	stream := w.ops.Stream(w.id)
	if stream == nil {
		return nil
	}
	spans, err := readWireSpans(stream, result)
	if err != nil {
		w.warn("the wire record is linked without service spans", err)
		return nil
	}
	return spans
}

func readWireSpans(stream *signal.Stream, result *contract.Result) ([]wire.Span, error) {
	runnerCalls := map[string]string{}
	if result != nil {
		for span, call := range signal.CallParents(*result) {
			runnerCalls[call] = strings.TrimPrefix(span, signal.W3CPrefix)
		}
	}
	var out []wire.Span
	for after := int64(0); ; {
		page, err := stream.Read(signal.ReadRequest{After: after})
		if err != nil {
			return nil, err
		}
		for _, sig := range page.Signals {
			if span, ok := wireSpan(sig, runnerCalls); ok {
				out = append(out, span)
			}
		}
		if page.Next <= after {
			return out, nil
		}
		after = page.Next
	}
}

func wireSpan(sig contract.Signal, runnerCalls map[string]string) (wire.Span, bool) {
	id, isW3C := strings.CutPrefix(sig.ID, signal.W3CPrefix)
	if sig.Collector != signal.OTLPCollector || sig.Span == nil || !isW3C {
		return wire.Span{}, false
	}
	parent, ok := runnerCalls[sig.Span.ParentID]
	if !ok {
		parent = strings.TrimPrefix(sig.Span.ParentID, signal.W3CPrefix)
	}
	start, err := time.Parse(time.RFC3339Nano, sig.Span.Start)
	if err != nil {
		return wire.Span{}, false
	}
	service, _ := sig.Attrs["service"].(string)
	return wire.NewSpan(id, parent, service, sig.Span.Name, sig.Attrs, start), true
}

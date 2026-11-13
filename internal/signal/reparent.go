package signal

import "github.com/d0lim/cockpit/internal/contract"

const W3CPrefix = "w3c:"

func CallParents(res contract.Result) map[string]string {
	out := map[string]string{}
	for _, st := range res.Steps {
		for i, c := range st.Calls {
			if c.SpanID != "" {
				out[W3CPrefix+c.SpanID] = callID(st, i)
			}
		}
	}
	return out
}

func Reparent(s *Stream, trace string, parents map[string]string) (int, error) {
	if len(parents) == 0 {
		return 0, nil
	}
	var moved []contract.Signal
	for after := int64(0); ; {
		page, err := s.Read(ReadRequest{After: after})
		if err != nil {
			return 0, err
		}
		for _, sig := range page.Signals {
			if sig.Collector != OTLPCollector || sig.Span == nil || sig.Span.TraceID != trace {
				continue
			}
			if call, ok := parents[sig.Span.ParentID]; ok {
				span := *sig.Span
				span.ParentID = call
				sig.Span, sig.Seq, sig.Rev = &span, 0, sig.Rev+1
				moved = append(moved, sig)
			}
		}
		if page.Next <= after {
			break
		}
		after = page.Next
	}
	for i, sig := range moved {
		if _, err := s.Append(sig); err != nil {
			return i, err
		}
	}
	return len(moved), nil
}

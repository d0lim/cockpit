package signal

import (
	"strconv"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
)

const RefPrefix = "ref."

func stepAttrs(st contract.ResultStep) contract.Attrs {
	attrs := contract.Attrs{"flow": st.Flow, "step": st.ID, "status": string(st.Status)}
	seen := map[string]int{}
	for _, r := range st.Refs {
		seen[r.Name]++
		key := RefPrefix + r.Name
		if n := seen[r.Name]; n > 1 {
			key += "." + strconv.Itoa(n)
		}
		attrs[key] = r.Value
	}
	return attrs
}

func callEnd(c contract.StepCall) string {
	t, err := time.Parse(time.RFC3339Nano, c.StartedAt)
	if err != nil {
		return c.StartedAt
	}
	return t.Add(time.Duration(c.DurationMs) * time.Millisecond).UTC().Format("2006-01-02T15:04:05.000Z")
}

func callSpans(root spanRoot, st contract.ResultStep) []contract.Signal {
	parent := "step:" + st.Flow + "/" + st.ID
	out := make([]contract.Signal, 0, len(st.Calls))
	for i, c := range st.Calls {
		attrs := contract.Attrs{"method": c.Method, "host": c.Host, "path": c.Path}
		status := contract.SpanStatusError
		if c.Status != nil {
			attrs["status"] = *c.Status
			if *c.Status < 400 {
				status = contract.SpanStatusOK
			}
		}
		id := "call:" + st.Flow + "/" + st.ID + "/" + strconv.Itoa(i+1)
		out = append(out, span(root, id, parent, c.Method+" "+c.Path, c.StartedAt, callEnd(c), status, attrs, nil))
	}
	return out
}

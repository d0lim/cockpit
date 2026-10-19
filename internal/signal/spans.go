package signal

import (
	"github.com/d0lim/cockpit/internal/contract"
)

const (
	RunSpanID  = "run"
	ExecSpanID = "exec"
)

type spanRoot struct {
	traceID string
	id      string
	name    string
	links   []contract.SpanLink
}

func stepStatus(s contract.StepStatus) contract.SpanStatus {
	switch s {
	case contract.StepStatusPassed:
		return contract.SpanStatusOK
	case contract.StepStatusFailed:
		return contract.SpanStatusError
	}
	return contract.SpanStatusIncomplete
}

func worse(a, b contract.SpanStatus) contract.SpanStatus {
	rank := map[contract.SpanStatus]int{contract.SpanStatusOK: 0, contract.SpanStatusIncomplete: 1, contract.SpanStatusError: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func span(root spanRoot, id, parent, name, start, end string, status contract.SpanStatus, attrs contract.Attrs, links []contract.SpanLink) contract.Signal {
	return contract.Signal{
		TS: start, TSSource: contract.TimeSourceRunner, Kind: contract.SignalKindSpan, Collector: RunnerSource, ID: id, Attrs: attrs,
		Span: &contract.SpanBody{TraceID: root.traceID, ParentID: parent, Name: name, Start: start, End: end, Status: status, Links: links},
	}
}

type flowSpan struct {
	start, end string
	status     contract.SpanStatus
}

func build(root spanRoot, res contract.Result, exitedAt string) []contract.Signal {
	var steps []contract.Signal
	var order []string
	flows := map[string]*flowSpan{}
	for _, st := range res.Steps {
		if st.StartedAt == "" {
			continue
		}
		status := stepStatus(st.Status)
		end := st.FinishedAt
		if end == "" || status == contract.SpanStatusIncomplete {
			end, status = exitedAt, contract.SpanStatusIncomplete
		}
		attrs := contract.Attrs{"flow": st.Flow, "step": st.ID, "status": string(st.Status)}
		steps = append(steps, span(root, "step:"+st.Flow+"/"+st.ID, "flow:"+st.Flow, st.ID, st.StartedAt, end, status, attrs, nil))
		f, ok := flows[st.Flow]
		if !ok {
			f = &flowSpan{start: st.StartedAt, end: end, status: contract.SpanStatusOK}
			flows[st.Flow] = f
			order = append(order, st.Flow)
		}
		f.start, f.end, f.status = min(f.start, st.StartedAt), max(f.end, end), worse(f.status, status)
	}
	runStatus := contract.SpanStatusOK
	out := make([]contract.Signal, 0, len(steps)+len(order)+1)
	for _, name := range order {
		f := flows[name]
		runStatus = worse(runStatus, f.status)
		out = append(out, span(root, "flow:"+name, root.id, name, f.start, f.end, f.status, contract.Attrs{"flow": name, "status": string(f.status)}, nil))
	}
	end := res.FinishedAt
	if !res.Complete {
		end, runStatus = exitedAt, contract.SpanStatusIncomplete
	}
	rootSpan := span(root, root.id, "", root.name, res.StartedAt, end, runStatus, contract.Attrs{"status": string(runStatus)}, root.links)
	return append(append([]contract.Signal{rootSpan}, out...), steps...)
}

func RunSpans(runID string, res contract.Result, exitedAt string) []contract.Signal {
	return build(spanRoot{traceID: runID, id: RunSpanID, name: res.Suite}, res, exitedAt)
}

func ExecSpans(runID, execName string, res contract.ExecResult, exitedAt string) []contract.Signal {
	root := spanRoot{
		traceID: runID + "/exec/" + execName,
		id:      ExecSpanID,
		name:    res.Target,
		links:   []contract.SpanLink{{TraceID: runID, SpanID: RunSpanID}},
	}
	return build(root, res.Result, exitedAt)
}

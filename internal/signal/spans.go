package signal

import (
	"github.com/d0lim/cockpit/internal/contract"
)

const (
	RunSpanID         = "run"
	ExecSpanID        = "exec"
	PipelineCollector = "pipeline"
	PipelineSpanID    = "pipeline"
	StageSpanPrefix   = "stage:"
	SuiteStageSpanID  = StageSpanPrefix + "suite"
)

type spanRoot struct {
	traceID string
	id      string
	parent  string
	name    string
	links   []contract.SpanLink
}

type Stage struct {
	Name   string
	Start  string
	End    string
	Status contract.SpanStatus
}

func hubSpan(sig contract.Signal) contract.Signal {
	sig.Collector, sig.TSSource = PipelineCollector, contract.TimeSourceHost
	return sig
}

func StageSpans(runID string, stages []Stage) []contract.Signal {
	if len(stages) == 0 {
		return nil
	}
	root := spanRoot{traceID: runID, id: PipelineSpanID, name: PipelineSpanID}
	whole := Stage{Start: stages[0].Start, End: stages[0].End, Status: contract.SpanStatusOK}
	out := make([]contract.Signal, 1, len(stages)+1)
	for _, st := range stages {
		whole.Start, whole.End, whole.Status = min(whole.Start, st.Start), max(whole.End, st.End), worse(whole.Status, st.Status)
		attrs := contract.Attrs{"stage": st.Name, "status": string(st.Status)}
		out = append(out, hubSpan(span(root, StageSpanPrefix+st.Name, root.id, st.Name, st.Start, st.End, st.Status, attrs, nil)))
	}
	out[0] = hubSpan(span(root, root.id, "", root.name, whole.Start, whole.End, whole.Status, contract.Attrs{"status": string(whole.Status)}, nil))
	return out
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
		steps = append(steps, span(root, "step:"+st.Flow+"/"+st.ID, "flow:"+st.Flow, st.ID, st.StartedAt, end, status, stepAttrs(st), nil))
		steps = append(steps, callSpans(root, st)...)
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
	rootSpan := span(root, root.id, root.parent, root.name, res.StartedAt, end, runStatus, contract.Attrs{"status": string(runStatus)}, root.links)
	return append(append([]contract.Signal{rootSpan}, out...), steps...)
}

func RunSpans(runID string, res contract.Result, exitedAt string) []contract.Signal {
	return build(spanRoot{traceID: runID, id: RunSpanID, parent: SuiteStageSpanID, name: res.Suite}, res, exitedAt)
}

func ExecTrace(runID, execName string) string { return runID + "/exec/" + execName }

func ExecSpans(runID, execName string, res contract.ExecResult, exitedAt string) []contract.Signal {
	root := spanRoot{
		traceID: ExecTrace(runID, execName),
		id:      ExecSpanID,
		name:    res.Target,
		links:   []contract.SpanLink{{TraceID: runID, SpanID: RunSpanID}},
	}
	return build(root, res.Result, exitedAt)
}

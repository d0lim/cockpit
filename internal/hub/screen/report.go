package screen

import (
	"encoding/json"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/labels"
)

func (st *state) report() (api.ReportView, error) { return st.s.d.Reader.Report(st.run.ID) }

func summaryOf(v api.ReportView) contract.Summary {
	var s contract.Summary
	if len(v.Summary) > 0 {
		json.Unmarshal(v.Summary, &s)
	}
	return s
}

func statusLabel(status string) string { return label(labelTable.FlowStatus, status) }

func flowTone(status string) Tone {
	switch status {
	case string(contract.FlowOutcomePassed):
		return ToneOK
	case string(contract.FlowOutcomeFailed):
		return ToneBad
	case string(contract.FlowOutcomeSkipped), string(contract.FlowOutcomeIncomplete), string(contract.StepStatusRunning):
		return ToneWarn
	}
	return ToneIdle
}

func count(p *int) string {
	if p == nil {
		return "-"
	}
	return strconv.Itoa(*p)
}

func notProvenItems(np *contract.NotProven) []string {
	if np == nil {
		return nil
	}
	var out []string
	for _, s := range np.Skipped {
		out = append(out, "skipped step · "+s)
	}
	for _, d := range np.Declared {
		out = append(out, "declared by the suite · "+d+" · "+statusLabel(labels.FlowNotProven))
	}
	for _, u := range np.Unread {
		out = append(out, "unread record · "+u)
	}
	return out
}

type failedStage struct {
	name   string
	detail string
}

func failedStages(s contract.Summary) []failedStage {
	var out []failedStage
	for name, stage := range s.Stages {
		if stage.Status == contract.CollectStageError {
			out = append(out, failedStage{name: name, detail: stage.Detail})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func requiresText(f contract.FlowSummary) string {
	if len(f.Requires) == 0 {
		return "-"
	}
	return strings.Join(f.Requires, " · ")
}

func stepFacts(s contract.FlowStep) string {
	facts := []string{"requests " + strconv.Itoa(s.Requests)}
	if s.DurationMs != nil {
		facts = append(facts, strconv.FormatInt(*s.DurationMs, 10)+"ms")
	}
	if s.HTTPStatus != nil {
		facts = append(facts, "HTTP "+strconv.Itoa(*s.HTTPStatus))
	}
	return strings.Join(facts, " · ")
}

func (st *state) flow() (api.FlowView, error) {
	return st.s.d.Reader.Flow(st.run.ID, st.values["flow"])
}

func pickedStep(f contract.FlowSummary, id string) *contract.FlowStep {
	for i := range f.StepList {
		if f.StepList[i].ID == id {
			return &f.StepList[i]
		}
	}
	return nil
}

type codeTarget struct {
	file  string
	line  int
	focus bool
}

func targetOf(f contract.FlowSummary, step *contract.FlowStep) *codeTarget {
	if step != nil && step.Source != nil {
		return &codeTarget{file: step.Source.File, line: step.Source.Line, focus: true}
	}
	if f.Source != nil {
		return &codeTarget{file: f.Source.File, line: f.Source.Line}
	}
	return nil
}

type flowRange struct{ start, end int }

func rangeOf(flows []contract.FlowSummary, flow, file string, lines int) *flowRange {
	type start struct {
		flow string
		line int
	}
	var starts []start
	for _, f := range flows {
		if f.Source != nil && f.Source.File == file && f.Source.Line <= lines {
			starts = append(starts, start{f.ID, f.Source.Line})
		}
	}
	slices.SortStableFunc(starts, func(a, b start) int { return a.line - b.line })
	for i, s := range starts {
		if s.flow != flow {
			continue
		}
		end := lines
		if i+1 < len(starts) {
			end = starts[i+1].line - 1
		}
		return &flowRange{start: s.line, end: max(s.line, end)}
	}
	return nil
}

type codeLine struct {
	n      int
	text   string
	inFlow bool
	here   bool
}

func codeLines(v api.SourceView, r *flowRange, here int) []codeLine {
	rows := strings.Split(v.Text, "\n")
	if len(rows) > v.Lines {
		rows = rows[:v.Lines]
	}
	out := make([]codeLine, len(rows))
	for i, text := range rows {
		n := i + 1
		out[i] = codeLine{n: n, text: text, inFlow: r != nil && n >= r.start && n <= r.end, here: n == here}
	}
	return out
}

func sourceBadge(source string) (Tone, string) {
	switch source {
	case api.SourceLive:
		return ToneOK, "run tree"
	case api.SourceBundle:
		return ToneInfo, "bundle copy"
	}
	return ToneIdle, source
}

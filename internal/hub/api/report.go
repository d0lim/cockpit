package api

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/labels"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/report"
)

const (
	ReportPending         = "pending"
	ReportRedactionFailed = "redaction-failed"
	ReportMetaOnly        = "meta-only"
	ReportBundle          = "bundle"
)

type ReportView struct {
	ID           string                 `json:"id"`
	Phase        lifecycle.Phase        `json:"phase"`
	PhaseLabel   string                 `json:"phaseLabel"`
	Verdict      lifecycle.Verdict      `json:"verdict"`
	VerdictLabel *string                `json:"verdictLabel"`
	Status       string                 `json:"status"`
	Files        []string               `json:"files,omitempty"`
	FailedStage  *lifecycle.Phase       `json:"failedStage,omitempty"`
	Notice       *string                `json:"notice,omitempty"`
	Tail         []string               `json:"tail,omitempty"`
	Summary      json.RawMessage        `json:"summary,omitempty"`
	Meta         json.RawMessage        `json:"meta,omitempty"`
	Flows        []contract.FlowSummary `json:"flows,omitempty"`
	Steps        *contract.StepCounts   `json:"steps,omitempty"`
	StoppedAt    *contract.SummaryStop  `json:"stoppedAt,omitempty"`
	NotProven    *contract.NotProven    `json:"notProven,omitempty"`
	ReportDir    string                 `json:"reportDir,omitempty"`
}

func reportView(runDir string, m store.Meta) ReportView {
	v := ReportView{ID: m.ID, Phase: m.Phase, PhaseLabel: lifecycle.Label(m.State), Verdict: m.Verdict, VerdictLabel: labels.Verdict(m.Verdict)}
	switch {
	case m.Mark == lifecycle.MarkNone && (m.Phase == lifecycle.Queued || m.Active()):
		v.Status = ReportPending
		return v
	case m.Redaction != nil && m.Redaction.Status == report.RedactionFailed:
		v.Status, v.Files = ReportRedactionFailed, m.Redaction.Files
		return v
	}
	dir := filepath.Join(runDir, report.DirReport)
	raw, failure := report.ReadConfined(dir, report.FileSummary)
	var summary contract.Summary
	if failure != report.ReadOK || json.Unmarshal(raw, &summary) != nil {
		stage, notice := m.FailedStage, m.Notice
		v.Status, v.FailedStage, v.Notice, v.Tail = ReportMetaOnly, &stage, &notice, m.Tail
		if v.Tail == nil {
			v.Tail = []string{}
		}
		return v
	}
	v.Status, v.Summary, v.ReportDir = ReportBundle, raw, dir
	if meta, f := report.ReadConfined(dir, report.FileMeta); f == report.ReadOK && json.Valid(meta) {
		v.Meta = meta
	}
	v.Flows, v.Steps, v.StoppedAt = summary.Flows, &summary.Steps, summary.StoppedAt
	if text, f := report.ReadConfined(dir, report.FileNotProven); f == report.ReadOK {
		np := notProvenOf(string(text))
		v.NotProven = &np
	}
	return v
}

func notProvenOf(text string) contract.NotProven {
	np := contract.NotProven{Skipped: []string{}, Declared: []string{}, Unread: []string{}}
	sections := map[string]*[]string{report.HeadingSkipped: &np.Skipped, report.HeadingDeclared: &np.Declared, report.HeadingUnread: &np.Unread}
	var into *[]string
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			into = sections[line]
		case into != nil && strings.HasPrefix(line, "- ") && line != "- none":
			*into = append(*into, line[2:])
		}
	}
	return np
}

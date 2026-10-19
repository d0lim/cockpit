package report

import (
	"fmt"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/contract"
)

const (
	HeadingSkipped  = "## Skipped steps"
	HeadingDeclared = "## Not proven as declared by the suite"
	HeadingUnread   = "## Unread records"
)

type Unread struct {
	Source string `json:"source"`
	Detail string `json:"detail,omitempty"`
	Count  int    `json:"count,omitempty"`
}

func UnreadLines(unread []Unread, dropped map[string]int64) []string {
	lines := []string{}
	for _, u := range unread {
		line := u.Source
		if u.Detail != "" {
			line += " — " + u.Detail
		}
		if u.Count > 1 {
			line += fmt.Sprintf(" (%d times)", u.Count)
		}
		lines = append(lines, line)
	}
	reasons := make([]string, 0, len(dropped))
	for reason, n := range dropped {
		if n > 0 {
			reasons = append(reasons, reason)
		}
	}
	slices.Sort(reasons)
	for _, reason := range reasons {
		lines = append(lines, fmt.Sprintf("signals dropped (%s): %d", reason, dropped[reason]))
	}
	return lines
}

func CollectorUnread(id string, o worker.Outcome) (contract.CollectStage, []Unread) {
	stage := contract.CollectStage{Status: contract.CollectStageOK}
	var out []Unread
	if o.Status != worker.StatusOK {
		detail := "collector " + string(o.Status)
		if o.Kind != "" {
			detail += ": " + o.Kind
		}
		stage = contract.CollectStage{Status: contract.CollectStageError, Detail: detail}
		out = append(out, Unread{Source: id, Detail: detail, Count: 1})
	}
	seen := map[worker.UnreadRecord]int{}
	var order []worker.UnreadRecord
	for _, r := range o.Unread {
		if seen[r] == 0 {
			order = append(order, r)
		}
		seen[r]++
	}
	for _, r := range order {
		out = append(out, Unread{Source: id + " · " + r.Source, Detail: r.Reason, Count: seen[r]})
	}
	return stage, out
}

func NotProvenOf(j Judgement, unread []string) contract.NotProven {
	return contract.NotProven{Skipped: j.Skipped, Declared: j.Declared, Unread: unread}
}

func NotProvenText(runID string, np contract.NotProven) string {
	var b strings.Builder
	b.WriteString("# Not proven — " + runID + "\n\n")
	section := func(heading string, items []string) {
		b.WriteString(heading + "\n\n")
		if len(items) == 0 {
			b.WriteString("- none\n")
		}
		for _, item := range items {
			b.WriteString("- " + item + "\n")
		}
		b.WriteString("\n")
	}
	section(HeadingSkipped, np.Skipped)
	section(HeadingDeclared, np.Declared)
	section(HeadingUnread, np.Unread)
	return b.String()
}

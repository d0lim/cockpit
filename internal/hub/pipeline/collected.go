package pipeline

import (
	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/report"
)

const stageCollectors = "collectors"

var collectStatus = map[collect.ReportStatus]contract.CollectStageStatus{
	collect.ReportOK:      contract.CollectStageOK,
	collect.ReportSkipped: contract.CollectStageSkipped,
	collect.ReportError:   contract.CollectStageError,
}

func collectorStages(in envops.ReportInput, stages map[string]contract.CollectStage) []report.Unread {
	mask := func(s string) string {
		if in.Masker == nil {
			return s
		}
		return in.Masker.Mask(s)
	}
	if in.CollectErr != nil && in.Collected == nil {
		detail := mask(in.CollectErr.Error())
		stages[stageCollectors] = contract.CollectStage{Status: contract.CollectStageError, Detail: detail}
		return []report.Unread{{Source: stageCollectors, Detail: detail, Count: 1}}
	}
	if in.Collected == nil {
		return nil
	}
	for _, c := range in.Collected.Collectors {
		status, ok := collectStatus[c.Status]
		if !ok {
			status = contract.CollectStageError
		}
		stages[c.ID] = contract.CollectStage{Status: status, Detail: mask(c.Reason)}
	}
	unread := make([]report.Unread, 0, len(in.Collected.Unread))
	for _, u := range in.Collected.Unread {
		source := u.Collector
		if u.Source != "" && u.Source != "collector:"+u.Collector {
			source += " · " + u.Source
		}
		unread = append(unread, report.Unread{Source: source, Detail: mask(u.Reason), Count: u.Count})
	}
	return unread
}

package pipeline

import (
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/signal"
)

const stageSpans = "spans"

func runSpans(in envops.ReportInput, res *contract.Result) []report.Unread {
	if in.Stream == nil || res == nil || res.SchemaVersion < contract.SchemaVersion2 || res.StartedAt == "" {
		return nil
	}
	exited := in.RunnerExit
	if exited.IsZero() {
		exited = in.Now
	}
	spans := signal.RunSpans(in.Meta.ID, *res, store.Timestamp(exited))
	for i, sig := range spans {
		if _, err := in.Stream.Append(sig); err != nil {
			detail := err.Error()
			if in.Masker != nil {
				detail = in.Masker.Mask(detail)
			}
			return []report.Unread{{Source: stageSpans, Detail: detail, Count: len(spans) - i}}
		}
	}
	return nil
}

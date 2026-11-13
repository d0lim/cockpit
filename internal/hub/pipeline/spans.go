package pipeline

import (
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/signal"
)

const stageSpansSource = "spans"

func reportSpans(in envops.ReportInput, res *contract.Result) []report.Unread {
	if in.Stream == nil {
		return nil
	}
	spans := stageSpans(in.Meta)
	traced := res != nil && res.SchemaVersion >= contract.SchemaVersion2 && res.StartedAt != ""
	if traced {
		exited := in.RunnerExit
		if exited.IsZero() {
			exited = in.Now
		}
		spans = append(spans, signal.RunSpans(in.Meta.ID, *res, store.Timestamp(exited))...)
	}
	for i, sig := range spans {
		if _, err := in.Stream.Append(sig); err != nil {
			return []report.Unread{{Source: stageSpansSource, Detail: maskedDetail(in, err), Count: len(spans) - i}}
		}
	}
	if traced {
		if _, err := signal.Reparent(in.Stream, in.Meta.ID, signal.CallParents(*res)); err != nil {
			return []report.Unread{{Source: stageSpansSource, Detail: maskedDetail(in, err), Count: 1}}
		}
	}
	return nil
}

func maskedDetail(in envops.ReportInput, err error) string {
	if in.Masker == nil {
		return err.Error()
	}
	return in.Masker.Mask(err.Error())
}

package pipeline

import (
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/signal"
)

const stageSpansSource = "spans"

func reportSpans(rec Recordings, m store.Meta, res *contract.Result, masker *mask.Masker) []report.Unread {
	if rec.Stream == nil {
		return nil
	}
	spans := stageSpans(m)
	traced := res != nil && res.SchemaVersion >= contract.SchemaVersion2 && res.StartedAt != ""
	if traced {
		exited := rec.Exited
		if exited.IsZero() {
			exited = rec.Now
		}
		spans = append(spans, signal.RunSpans(m.ID, *res, store.Timestamp(exited))...)
	}
	for i, sig := range spans {
		if _, err := rec.Stream.Append(sig); err != nil {
			return []report.Unread{{Source: stageSpansSource, Detail: maskedDetail(masker, err), Count: len(spans) - i}}
		}
	}
	if traced {
		if _, err := signal.Reparent(rec.Stream, m.ID, signal.CallParents(*res)); err != nil {
			return []report.Unread{{Source: stageSpansSource, Detail: maskedDetail(masker, err), Count: 1}}
		}
	}
	return nil
}

func maskedDetail(m *mask.Masker, err error) string {
	if m == nil {
		return err.Error()
	}
	return m.Mask(err.Error())
}

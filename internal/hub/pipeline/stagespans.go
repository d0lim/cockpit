package pipeline

import (
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/signal"
)

var stageNames = map[lifecycle.Phase]string{
	lifecycle.Preparing: "prepare",
	lifecycle.Building:  "build",
	lifecycle.Sealing:   "seal",
	lifecycle.Booting:   "boot",
	lifecycle.Testing:   "suite",
	lifecycle.Reporting: "report",
}

var stageEndStatus = map[lifecycle.Event]contract.SpanStatus{
	lifecycle.Fail:  contract.SpanStatusError,
	lifecycle.Abort: contract.SpanStatusIncomplete,
}

func stageSpans(m store.Meta) []contract.Signal {
	var ended []signal.Stage
	for _, st := range m.Stages {
		if name, ok := stageNames[st.Stage]; ok && st.End != "" {
			ended = append(ended, signal.Stage{Name: name, Start: st.Start, End: st.End, Status: st.Outcome})
		}
	}
	return signal.StageSpans(m.ID, ended)
}

func (l *live) startStage(p lifecycle.Phase) error {
	at := store.Timestamp(l.h.o.Now())
	_, err := l.st.PatchMeta(l.id, func(m *store.Meta) { m.StartStage(p, at) })
	return err
}

func (l *live) endingStage(out outcome) func(*store.Meta) {
	status, failed := stageEndStatus[out.event]
	if !failed {
		status = contract.SpanStatusOK
	}
	at := store.Timestamp(l.h.o.Now())
	return func(m *store.Meta) {
		if out.extra != nil {
			out.extra(m)
		}
		m.EndStage(at, status)
	}
}

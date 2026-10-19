package report

import (
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
)

func Settle(j Judgement, preset lifecycle.Verdict) Judgement {
	if preset == lifecycle.VerdictNone || contract.FinalVerdict(preset) == j.Verdict {
		return j
	}
	reason := "the verdict was set before the report (" + string(preset) + "); the rules give " + string(j.Verdict)
	j.Reasons = append([]string{reason}, j.Reasons...)
	j.Verdict = contract.FinalVerdict(preset)
	return j
}

func BundleMetaOf(m store.Meta, s SummaryInput, verdict contract.FinalVerdict) contract.BundleMeta {
	b := contract.BundleMeta{
		ID:             m.ID,
		SHA:            m.SHA,
		Suite:          m.Suite,
		Harness:        s.Harness,
		HubCommit:      s.HubCommit,
		Submitter:      m.Submitter,
		Keep:           m.Keep,
		CreatedAt:      m.CreatedAt,
		StartedAt:      m.StartedAt,
		FinishedAt:     m.FinishedAt,
		Slot:           m.Slot,
		ImageKeys:      m.ImageKeys,
		ImageDigests:   m.ImageDigests,
		FailedStage:    string(m.FailedStage),
		Notice:         m.Notice,
		Tail:           m.Tail,
		Verdict:        verdict,
		ReportedAt:     s.GeneratedAt,
		CockpitVersion: m.CockpitVersion,
		HostAPI:        m.HostAPI,
	}
	if m.RunnerImage != nil {
		b.RunnerImage = *m.RunnerImage
	}
	return b
}

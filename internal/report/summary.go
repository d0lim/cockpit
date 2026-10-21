package report

import (
	"github.com/d0lim/cockpit/internal/contract"
)

type SummaryInput struct {
	RunID          string
	Suite          string
	Commit         string
	Harness        int
	HubCommit      string
	HostAPI        int
	CockpitVersion string
	RunnerImage    contract.RunnerImage
	Ports          map[string]int
	ImageKeys      map[string]string
	ImageDigests   map[string]string
	Versions       map[string]string
	Stages         map[string]contract.CollectStage
	Runner         *contract.RunnerOutcome
	GeneratedAt    string
}

func orEmpty[V any](m map[string]V) map[string]V {
	if m == nil {
		return map[string]V{}
	}
	return m
}

func Summary(in Inputs, j Judgement, s SummaryInput, unread []string) contract.Summary {
	return contract.Summary{
		Format:         contract.SummaryFormat3,
		RunID:          s.RunID,
		Suite:          s.Suite,
		Verdict:        j.Verdict,
		Reasons:        j.Reasons,
		Steps:          StepCountsOf(in.Manifest, in.Result),
		Flows:          FlowSummaries(in.Manifest, in.Result),
		StoppedAt:      StopPoint(in.Result),
		NotProven:      NotProvenOf(j, unread),
		Boot:           in.Boot,
		Seal:           in.Seal,
		Runner:         s.Runner,
		Stages:         orEmpty(s.Stages),
		Commit:         s.Commit,
		Harness:        s.Harness,
		HubCommit:      s.HubCommit,
		Ports:          orEmpty(s.Ports),
		Images:         contract.SummaryImages{Keys: orEmpty(s.ImageKeys), Digests: orEmpty(s.ImageDigests)},
		Versions:       orEmpty(s.Versions),
		CockpitVersion: s.CockpitVersion,
		HostAPI:        s.HostAPI,
		RunnerImage:    s.RunnerImage,
		GeneratedAt:    s.GeneratedAt,
	}
}

func StepCountsOf(m ManifestRead, r ResultRead) contract.StepCounts {
	var c contract.StepCounts
	if m.Value != nil {
		n := 0
		for _, f := range m.Value.Flows {
			n += len(f.Steps)
		}
		c.Expected = &n
	}
	if r.Value != nil {
		n := len(r.Value.Steps)
		c.Actual = &n
		for _, s := range r.Value.Steps {
			c.Requests += s.Requests
		}
		c.Failed, c.Skipped = counts(r.Value)
	}
	return c
}

func FlowSummaries(m ManifestRead, r ResultRead) []contract.FlowSummary {
	out := []contract.FlowSummary{}
	if m.Value == nil {
		return out
	}
	byKey := map[string]contract.ResultStep{}
	if r.Value != nil {
		for _, s := range r.Value.Steps {
			byKey[stepKey(s.Flow, s.ID)] = s
		}
	}
	for _, f := range m.Value.Flows {
		fs := contract.FlowSummary{ID: f.ID, Stage: f.Stage, Title: f.Title, Once: f.Once, Requires: f.Requires, Source: f.Source, Steps: len(f.Steps), StepList: []contract.FlowStep{}}
		if fs.Requires == nil {
			fs.Requires = []string{}
		}
		var results []*contract.ResultStep
		for _, s := range f.Steps {
			step := contract.FlowStep{ID: s.ID, Title: s.Title, Kind: s.Kind, Status: contract.StepStatusNotRun, Source: s.Source}
			res, ok := byKey[stepKey(f.ID, s.ID)]
			if ok {
				step.Status, step.Requests, step.DurationMs, step.HTTPStatus, step.Message = res.Status, res.Requests, res.DurationMs, res.HTTPStatus, res.Message
				step.Calls, step.Refs = res.Calls, res.Refs
				results = append(results, &res)
			} else {
				results = append(results, nil)
			}
			switch step.Status {
			case contract.StepStatusPassed:
				fs.Passed++
			case contract.StepStatusFailed:
				fs.Failed++
			case contract.StepStatusSkipped:
				fs.Skipped++
			}
			fs.StepList = append(fs.StepList, step)
		}
		fs.Status, fs.StoppedAt = flowOutcome(results)
		out = append(out, fs)
	}
	return out
}

func flowOutcome(steps []*contract.ResultStep) (contract.FlowOutcome, string) {
	for _, s := range steps {
		if s != nil && s.Status == contract.StepStatusFailed {
			return contract.FlowOutcomeFailed, s.ID
		}
	}
	for i := len(steps) - 1; i >= 0; i-- {
		if s := steps[i]; s != nil && s.Status == contract.StepStatusRunning {
			return contract.FlowOutcomeIncomplete, s.ID
		}
	}
	all := func(st contract.StepStatus) bool {
		for _, s := range steps {
			if s == nil || s.Status != st {
				return false
			}
		}
		return len(steps) > 0
	}
	switch {
	case all(contract.StepStatusPassed):
		return contract.FlowOutcomePassed, ""
	case all(contract.StepStatusSkipped):
		return contract.FlowOutcomeSkipped, ""
	}
	for _, s := range steps {
		if s != nil && s.Status != contract.StepStatusNotRun {
			return contract.FlowOutcomeIncomplete, ""
		}
	}
	return contract.FlowOutcomeNotRun, ""
}

package report

import (
	"fmt"

	"github.com/d0lim/cockpit/internal/contract"
)

type ReadFailure string

const (
	ReadOK      ReadFailure = ""
	ReadMissing ReadFailure = "missing"
	ReadUnsafe  ReadFailure = "unsafe"
	ReadParse   ReadFailure = "parse"
	ReadSchema  ReadFailure = "schema"
)

var readText = map[ReadFailure]string{
	ReadUnsafe: "could not be read safely",
	ReadParse:  "is not JSON",
	ReadSchema: "does not match the schema",
}

type ManifestRead struct {
	Value   *contract.Manifest
	Failure ReadFailure
}

type ResultRead struct {
	Value   *contract.Result
	Failure ReadFailure
}

type Inputs struct {
	Manifest   ManifestRead
	Result     ResultRead
	Boot       *contract.BootVerdict
	Seal       contract.SealVerdict
	RunnerCode *int
	OOMKilled  bool
}

type Judgement struct {
	Verdict  contract.FinalVerdict
	Rule     int
	Reasons  []string
	Skipped  []string
	Declared []string
}

type finding struct {
	verdict contract.FinalVerdict
	reason  string
}

type rule struct {
	Number    int
	Condition string
	Verdicts  string
	check     func(in Inputs) *finding
}

func stepKey(flow, step string) string { return flow + " › " + step }

func fail(v contract.FinalVerdict, format string, args ...any) *finding {
	return &finding{verdict: v, reason: fmt.Sprintf(format, args...)}
}

func Judge(in Inputs) Judgement {
	return judgeWith(rules, in)
}

func judgeWith(list []rule, in Inputs) Judgement {
	j := Judgement{Verdict: contract.VerdictPassed, Reasons: []string{}}
	j.Skipped, j.Declared = unproven(in)
	for _, r := range list {
		f := r.check(in)
		if f == nil {
			continue
		}
		j.Verdict, j.Rule, j.Reasons = f.verdict, r.Number, []string{f.reason}
		if stop := StopPoint(in.Result); stop != nil {
			line := stepKey(stop.Flow, stop.Step)
			if stop.Message != "" {
				line += " — " + stop.Message
			}
			j.Reasons = append(j.Reasons, line)
		}
		return j
	}
	return j
}

func StopPoint(res ResultRead) *contract.SummaryStop {
	if res.Value == nil {
		return nil
	}
	r := res.Value
	var flow, step string
	if !r.Complete {
		for i := len(r.Steps) - 1; i >= 0; i-- {
			if r.Steps[i].Status == contract.StepStatusRunning {
				flow, step = r.Steps[i].Flow, r.Steps[i].ID
				break
			}
		}
	}
	if step == "" && r.StoppedAt != nil {
		flow, step = r.StoppedAt.Flow, r.StoppedAt.Step
	}
	if step == "" {
		return nil
	}
	stop := &contract.SummaryStop{Flow: flow, Step: step}
	for _, s := range r.Steps {
		if s.Flow == flow && s.ID == step {
			stop.Message = s.Message
		}
	}
	return stop
}

func unproven(in Inputs) (skipped, declared []string) {
	skipped, declared = []string{}, []string{}
	if in.Manifest.Value != nil {
		for _, s := range in.Manifest.Value.Stages {
			if s.NotProven != "" {
				declared = append(declared, s.Title+" — "+s.NotProven)
			}
		}
	}
	if in.Result.Value != nil {
		for _, s := range in.Result.Value.Steps {
			if s.Status == contract.StepStatusSkipped {
				skipped = append(skipped, stepKey(s.Flow, s.ID))
			}
		}
	}
	return skipped, declared
}

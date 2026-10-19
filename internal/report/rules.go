package report

import (
	"github.com/d0lim/cockpit/internal/contract"
)

const (
	vFailed = contract.VerdictFailed
	vError  = contract.VerdictError
)

var rules = []rule{
	{1, "A seal violation exists, or the seal comparison could not be made", "failed, error", sealRule},
	{2, "The expected services did not all come up", "error", bootRule},
	{3, "The runner was killed for lack of memory", "error", func(in Inputs) *finding {
		if in.OOMKilled {
			return fail(vError, "the runner was killed for lack of memory (OOMKilled)")
		}
		return nil
	}},
	{4, "There is no runner exit code (time limit exceeded or aborted)", "error", func(in Inputs) *finding {
		if in.RunnerCode == nil {
			return fail(vError, "no runner exit code was received: time limit exceeded or aborted")
		}
		return nil
	}},
	{5, "The runner exit code is not 0, 1 or 2", "error", func(in Inputs) *finding {
		if c, ok := code(in); ok && c != 0 && c != 1 && c != 2 {
			known := map[int]string{137: "SIGKILL or OOM"}[c]
			if known == "" {
				known = "outside the contract (0, 1, 2)"
			}
			return fail(vError, "runner exit code %d: %s", c, known)
		}
		return nil
	}},
	{6, "The manifest is missing or could not be read", "error", func(in Inputs) *finding {
		switch in.Manifest.Failure {
		case ReadOK:
			return nil
		case ReadMissing:
			return fail(vError, "the manifest is missing")
		}
		return fail(vError, "the manifest %s", readText[in.Manifest.Failure])
	}},
	{7, "The result is missing, has `complete: false`, or does not match the schema", "error", func(in Inputs) *finding {
		switch in.Result.Failure {
		case ReadOK:
		case ReadMissing:
			return fail(vError, "the result is missing")
		default:
			return fail(vError, "the result %s", readText[in.Result.Failure])
		}
		if in.Result.Value == nil || !in.Result.Value.Complete {
			return fail(vError, "the result did not finish (complete: false)")
		}
		return nil
	}},
	{8, "The step ids of the result differ from the manifest or repeat", "error", wiringRule},
	{9, "There are no flows or no steps", "error", func(in Inputs) *finding {
		if !ready(in) {
			return nil
		}
		if len(in.Manifest.Value.Flows) == 0 {
			return fail(vError, "the manifest has no flows")
		}
		if len(in.Result.Value.Steps) == 0 {
			return fail(vError, "the manifest has no steps")
		}
		return nil
	}},
	{10, "A finished result still has steps that did not run", "error", func(in Inputs) *finding {
		if !ready(in) {
			return nil
		}
		for _, s := range in.Result.Value.Steps {
			if s.Status == contract.StepStatusNotRun || s.Status == contract.StepStatusRunning {
				return fail(vError, "a step did not run in a finished result: %s", stepKey(s.Flow, s.ID))
			}
		}
		return nil
	}},
	{11, "A `request` step passed but sent no request (a vacuous step)", "error", vacuousRule},
	{12, "The exit code is 0 but there are failed or skipped steps", "error", func(in Inputs) *finding {
		c, ok := code(in)
		if !ok || !ready(in) {
			return nil
		}
		failed, skipped := counts(in.Result.Value)
		if c == 0 && failed+skipped > 0 {
			return fail(vError, "exit code 0 with failed or skipped steps: the runner and the result disagree")
		}
		return nil
	}},
	{13, "A step failed", "failed", func(in Inputs) *finding {
		if !ready(in) {
			return nil
		}
		failed, _ := counts(in.Result.Value)
		if failed == 0 {
			return nil
		}
		for _, s := range in.Result.Value.Steps {
			if s.Status == contract.StepStatusFailed {
				return fail(vFailed, "failed steps: %d, the first at %s", failed, stepKey(s.Flow, s.ID))
			}
		}
		return nil
	}},
	{14, "A step was skipped without a failed ancestor (a wrong prerequisite)", "error", orphanRule},
	{15, "The exit code is 2, or the exit code is 1 with no failed or skipped steps", "error", func(in Inputs) *finding {
		c, ok := code(in)
		if !ok || !ready(in) {
			return nil
		}
		_, skipped := counts(in.Result.Value)
		switch {
		case c == 2:
			return fail(vError, "runner internal error (exit code 2)")
		case c == 1 && skipped == 0:
			return fail(vError, "exit code 1 with no failed or skipped steps: the runner and the result disagree")
		}
		return nil
	}},
}

func sealRule(in Inputs) *finding {
	switch in.Seal {
	case contract.SealPassed:
		return nil
	case contract.SealFailed:
		return fail(vFailed, "the seal check failed: a seal violation exists (seal.json)")
	}
	return fail(vError, "the seal check could not decide: the comparison did not hold (seal.json)")
}

func bootRule(in Inputs) *finding {
	if in.Boot == nil {
		return fail(vError, "there is no boot verdict: the run closed before booting")
	}
	if in.Boot.OK {
		return nil
	}
	pending := in.Boot.Pending != nil && *in.Boot.Pending
	first := "no reason given"
	switch {
	case pending && len(in.Boot.Waiting) > 0:
		first = in.Boot.Waiting[0]
	case pending:
		first = "some services are not ready"
	case len(in.Boot.Reasons) > 0:
		first = in.Boot.Reasons[0]
	}
	return fail(vError, "the boot check failed: %s", first)
}

func ready(in Inputs) bool { return in.Manifest.Value != nil && in.Result.Value != nil }

func code(in Inputs) (int, bool) {
	if in.RunnerCode == nil {
		return 0, false
	}
	return *in.RunnerCode, true
}

func counts(r *contract.Result) (failed, skipped int) {
	for _, s := range r.Steps {
		switch s.Status {
		case contract.StepStatusFailed:
			failed++
		case contract.StepStatusSkipped:
			skipped++
		}
	}
	return failed, skipped
}

func wiringRule(in Inputs) *finding {
	if !ready(in) {
		return nil
	}
	expected := map[string]bool{}
	for _, f := range in.Manifest.Value.Flows {
		for _, s := range f.Steps {
			k := stepKey(f.ID, s.ID)
			if expected[k] {
				return fail(vError, "a step id repeats in the manifest: %s", k)
			}
			expected[k] = true
		}
	}
	actual := map[string]bool{}
	for _, s := range in.Result.Value.Steps {
		k := stepKey(s.Flow, s.ID)
		if actual[k] {
			return fail(vError, "a step id repeats in the result: %s", k)
		}
		actual[k] = true
	}
	if len(actual) != len(expected) {
		return fail(vError, "the step counts differ: result %d, manifest %d", len(actual), len(expected))
	}
	for _, s := range in.Result.Value.Steps {
		if !expected[stepKey(s.Flow, s.ID)] {
			return fail(vError, "the result has a step that is not in the manifest: %s", stepKey(s.Flow, s.ID))
		}
	}
	return nil
}

func vacuousRule(in Inputs) *finding {
	if !ready(in) {
		return nil
	}
	kinds := map[string]contract.StepKind{}
	for _, f := range in.Manifest.Value.Flows {
		for _, s := range f.Steps {
			kinds[stepKey(f.ID, s.ID)] = s.Kind
		}
	}
	for _, s := range in.Result.Value.Steps {
		if s.Status == contract.StepStatusPassed && s.Requests == 0 && kinds[stepKey(s.Flow, s.ID)] == contract.StepKindRequest {
			return fail(vError, "no request was sent: %s", stepKey(s.Flow, s.ID))
		}
	}
	return nil
}

func orphanRule(in Inputs) *finding {
	if !ready(in) {
		return nil
	}
	blocked := map[string]bool{}
	for _, s := range in.Result.Value.Steps {
		if s.Status == contract.StepStatusFailed {
			blocked[s.Flow] = true
		}
	}
	for _, f := range in.Manifest.Value.Flows {
		for _, r := range f.Requires {
			if blocked[r] {
				blocked[f.ID] = true
			}
		}
	}
	for _, s := range in.Result.Value.Steps {
		if s.Status == contract.StepStatusSkipped && !blocked[s.Flow] {
			return fail(vError, "wrong prerequisite: %s was skipped without a failed ancestor", stepKey(s.Flow, s.ID))
		}
	}
	return nil
}

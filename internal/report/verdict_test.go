package report

import (
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

func TestAllFifteenRulesGiveTheirVerdictAndReasonFromFixedInputs(t *testing.T) {
	covered := map[int]bool{}
	cases := ruleCases()
	for _, c := range cases {
		j := Judge(c.in)
		if j.Verdict != c.verdict || j.Rule != c.rule || len(j.Reasons) == 0 || !strings.Contains(j.Reasons[0], c.reason) {
			t.Errorf("%s: got %s by rule %d %q, want %s by rule %d containing %q", c.name, j.Verdict, j.Rule, j.Reasons, c.verdict, c.rule, c.reason)
		}
		covered[c.rule] = true
	}
	if len(rules) != 15 || len(covered) != 15 || len(cases) != 24 {
		t.Fatalf("rules %d, covered %d, cases %d", len(rules), len(covered), len(cases))
	}
	for i, r := range rules {
		if r.Number != i+1 {
			t.Fatalf("rule at %d is numbered %d", i, r.Number)
		}
	}
}

func TestAPassingRunKeepsTheDeclaredNotProvenWithoutBlockingTheVerdict(t *testing.T) {
	j := Judge(base())
	if j.Verdict != contract.VerdictPassed || j.Rule != 0 || len(j.Reasons) != 0 || len(j.Skipped) != 0 {
		t.Fatalf("judgement %+v", j)
	}
	if !slices.Equal(j.Declared, []string{declaredLine}) {
		t.Fatalf("declared %q", j.Declared)
	}
}

func TestReversedRuleOrderTurnsAtLeastTwoCasesRed(t *testing.T) {
	reversed := slices.Clone(rules)
	slices.Reverse(reversed)
	if red := redUnder(reversed); red < 2 {
		t.Fatalf("reversed order turned %d cases red", red)
	}
}

func TestSwappingEachOverlappingPairOfNeighbouringRulesTurnsACaseRed(t *testing.T) {
	pairs := [][2]int{{1, 2}, {2, 3}, {3, 4}, {5, 6}, {6, 7}, {7, 8}, {8, 9}, {10, 11}, {11, 12}, {12, 13}, {13, 14}, {14, 15}}
	for _, p := range pairs {
		swapped := slices.Clone(rules)
		swapped[p[0]-1], swapped[p[1]-1] = swapped[p[1]-1], swapped[p[0]-1]
		if redUnder(swapped) == 0 {
			t.Errorf("swapping rules %d and %d turned no case red", p[0], p[1])
		}
	}
	if len(pairs) != 12 {
		t.Fatal("pair count")
	}
}

func redUnder(list []rule) int {
	red := 0
	for _, c := range ruleCases() {
		j := judgeWith(list, c.in)
		if j.Verdict != c.verdict || j.Rule != c.rule {
			red++
		}
	}
	return red
}

func TestStepSetMismatchUnfinishedResultAndZeroRequestPassedStepAreEachAnError(t *testing.T) {
	cases := map[string]Inputs{
		"step-set mismatch": with(func(in *Inputs) { in.Result.Value.Steps = in.Result.Value.Steps[:5] }),
		"complete false":    with(func(in *Inputs) { in.Result.Value.Complete = false }),
		"zero-request pass": with(func(in *Inputs) { step(in.Result.Value, "alpha", "a1").Requests = 0 }),
	}
	for name, in := range cases {
		if j := Judge(in); j.Verdict != contract.VerdictError {
			t.Errorf("%s: %s %q", name, j.Verdict, j.Reasons)
		}
	}
	allow := with(func(in *Inputs) { step(in.Result.Value, "alpha", "a3").Requests = 0 })
	if j := Judge(allow); j.Verdict != contract.VerdictPassed {
		t.Fatalf("a wait step with zero requests is not vacuous: %s %q", j.Verdict, j.Reasons)
	}
	if len(cases) != 3 {
		t.Fatal("case count")
	}
}

func TestTheSecondReasonIsTheStopPointWithTheStepMessageAndTheFirstCarriesOnlyIDs(t *testing.T) {
	j := Judge(with(func(in *Inputs) {
		in.Result.Value, in.RunnerCode = failedAtA2("status 500, want 201 "+planted), intp(1)
	}))
	if j.Verdict != contract.VerdictFailed || len(j.Reasons) != 2 {
		t.Fatalf("judgement %+v", j)
	}
	if strings.Contains(j.Reasons[0], planted) || j.Reasons[1] != "alpha › a2 — status 500, want 201 "+planted {
		t.Fatalf("reasons %q", j.Reasons)
	}
	if !slices.Equal(j.Skipped, []string{"alpha › a3", "beta › b1", "beta › b2"}) {
		t.Fatalf("skipped %q", j.Skipped)
	}
}

func TestAnUnfinishedResultStopsAtTheLastRunningStep(t *testing.T) {
	j := Judge(with(func(in *Inputs) {
		in.Result.Value.Complete = false
		step(in.Result.Value, "beta", "b1").Status = contract.StepStatusRunning
		step(in.Result.Value, "beta", "b2").Status = contract.StepStatusNotRun
		step(in.Result.Value, "gamma", "g1").Status = contract.StepStatusNotRun
		in.RunnerCode = nil
	}))
	if j.Rule != 4 || len(j.Reasons) != 2 || j.Reasons[1] != "beta › b1" {
		t.Fatalf("judgement %+v", j)
	}
}

func TestAVerdictSetBeforeTheReportWinsAndTheRuleVerdictIsKeptAsTheFirstReason(t *testing.T) {
	j := Settle(Judge(base()), lifecycle.VerdictAborted)
	if j.Verdict != contract.VerdictAborted || !strings.Contains(j.Reasons[0], "the rules give passed") {
		t.Fatalf("judgement %+v", j)
	}
	same := Settle(Judge(base()), lifecycle.VerdictPassed)
	if same.Verdict != contract.VerdictPassed || len(same.Reasons) != 0 {
		t.Fatalf("same verdict changed the reasons: %+v", same)
	}
}

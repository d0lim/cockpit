package runner

import (
	"fmt"
	"slices"

	"github.com/d0lim/cockpit/internal/contract"
)

type ExecPlan struct {
	Target string
	Flows  []FlowDecl
}

type Rejection struct {
	Reason string
}

func (r *Rejection) Error() string { return r.Reason }

func PlanExec(ordered []FlowDecl, targets []string, ledger contract.Ledger) ([]ExecPlan, error) {
	byID := map[string]FlowDecl{}
	position := map[string]int{}
	for i, f := range ordered {
		byID[f.ID] = f
		position[f.ID] = i
	}
	passed := map[string]bool{}
	ran := map[string]bool{}
	for id, e := range ledger.Flows {
		if e.Status == contract.FlowStatusPassed {
			passed[id] = true
		}
		if e.Status != contract.FlowStatusSkipped {
			ran[id] = true
		}
	}
	var plans []ExecPlan
	for _, target := range targets {
		flow, ok := byID[target]
		if !ok {
			return nil, &Rejection{Reason: fmt.Sprintf("flow %s is not in the suite", target)}
		}
		if flow.Once && ran[target] {
			return nil, &Rejection{Reason: target + " is a once-per-environment flow and already ran in this environment"}
		}
		rerun := map[string]bool{}
		stack := slices.Clone(flow.Requires)
		for len(stack) > 0 {
			id := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			pre := byID[id]
			if pre.Once {
				if !passed[id] {
					return nil, &Rejection{Reason: fmt.Sprintf("%s requires %s, which has never passed in this environment", target, id)}
				}
				continue
			}
			if rerun[id] {
				continue
			}
			rerun[id] = true
			stack = append(stack, pre.Requires...)
		}
		ids := make([]string, 0, len(rerun))
		for id := range rerun {
			ids = append(ids, id)
		}
		slices.SortFunc(ids, func(a, b string) int { return position[a] - position[b] })
		flows := make([]FlowDecl, 0, len(ids)+1)
		for _, id := range ids {
			flows = append(flows, byID[id])
		}
		plans = append(plans, ExecPlan{Target: target, Flows: append(flows, flow)})
		if flow.Once {
			ran[target] = true
		}
		passed[target] = true
	}
	return plans, nil
}

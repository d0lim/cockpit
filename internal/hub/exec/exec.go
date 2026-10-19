package exec

import (
	"context"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/pool"
	"github.com/d0lim/cockpit/internal/hub/runfiles"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/signal"
)

var FlowID = regexp.MustCompile(`^[a-z][a-z0-9-]{1,40}$`)

type Execer struct {
	Ops *envops.Ops
	Now func() time.Time
}

type StepView struct {
	ID      string              `json:"id"`
	Status  contract.StepStatus `json:"status"`
	Message string              `json:"message,omitempty"`
}

type FlowView struct {
	ID    string     `json:"id"`
	Steps []StepView `json:"steps"`
}

type View struct {
	ExitCode  *int       `json:"exitCode"`
	OOMKilled bool       `json:"oomKilled"`
	TimedOut  bool       `json:"timedOut"`
	Flows     []FlowView `json:"flows"`
	Lines     []string   `json:"lines"`
	Reason    string     `json:"reason,omitempty"`
}

func conflict(m store.Meta, reason string) *ticker.Failure {
	return &ticker.Failure{Status: 409, Code: ticker.CodeConflictState, Reason: reason, Run: &m}
}

func gate(m store.Meta, busy bool) *ticker.Failure {
	if !pool.Stoppable(m) {
		hint := ""
		if m.Mark == lifecycle.MarkNone && m.Phase == lifecycle.Done && m.Env == lifecycle.EnvStopped {
			hint = "; resume it first"
		}
		return conflict(m, "exec runs only in a kept environment that is up; this run is "+lifecycle.Label(m.State)+hint)
	}
	if busy {
		return conflict(m, "an operation is in progress on this run")
	}
	return nil
}

func unmetOnce(flows map[string]contract.ManifestFlow, ledger *contract.Ledger, target contract.ManifestFlow) string {
	seen := map[string]bool{}
	var walk func(f contract.ManifestFlow) string
	walk = func(f contract.ManifestFlow) string {
		for _, id := range f.Requires {
			if seen[id] {
				continue
			}
			seen[id] = true
			req, ok := flows[id]
			if !ok {
				return id
			}
			if req.Once {
				if ledger.Flows[id].Status != contract.FlowStatusPassed {
					return id
				}
				continue
			}
			if deeper := walk(req); deeper != "" {
				return deeper
			}
		}
		return ""
	}
	return walk(target)
}

func Decide(m store.Meta, manifest *contract.Manifest, ledger *contract.Ledger, flow string) *ticker.Failure {
	flows := map[string]contract.ManifestFlow{}
	for _, f := range manifest.Flows {
		flows[f.ID] = f
	}
	target, ok := flows[flow]
	if !ok {
		return &ticker.Failure{Status: 404, Code: ticker.CodeNotFound, Reason: "the manifest of this run has no flow " + flow, Run: &m}
	}
	switch ledger.Flows[flow].Status {
	case contract.FlowStatusStarted, contract.FlowStatusPassed, contract.FlowStatusFailed:
		if target.Once {
			return conflict(m, fmt.Sprintf("%s runs once per environment and already ran here (%s); submit a new run", flow, ledger.Flows[flow].Status))
		}
	}
	if unmet := unmetOnce(flows, ledger, target); unmet != "" {
		return conflict(m, "the prerequisite "+unmet+" has not passed in this environment")
	}
	return nil
}

func (x *Execer) Denied(m store.Meta, busy bool, flow string) *ticker.Failure {
	if denied := gate(m, busy); denied != nil {
		return denied
	}
	if !FlowID.MatchString(flow) {
		return &ticker.Failure{Status: 400, Code: ticker.CodeBadRequest, Reason: "not a flow id: it must match " + FlowID.String(), Run: &m}
	}
	dir, err := x.Ops.Store.RunDir(m.ID)
	if err != nil {
		return conflict(m, err.Error())
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return conflict(m, "the run directory cannot be opened")
	}
	defer root.Close()
	manifest, err := runfiles.Manifest(root)
	if err != nil {
		return conflict(m, "the manifest cannot be read: "+err.Error())
	}
	ledger, err := runfiles.Ledger(root)
	if err != nil {
		return conflict(m, "the ledger cannot be read: "+err.Error())
	}
	return Decide(m, manifest, ledger, flow)
}

func OutName(now time.Time, flow string) string {
	return strings.NewReplacer(":", "-", ".", "-").Replace(now.UTC().Format("2006-01-02T15:04:05.000Z")) + "-" + flow + ".json"
}

func (x *Execer) Run(ctx context.Context, m store.Meta, flow string) scheduler.ExecResponse {
	fail := func(reason string) scheduler.ExecResponse {
		return scheduler.ExecResponse{Fail: &ticker.Failure{Status: 500, Code: ticker.CodeOpFailed, Reason: reason, Run: &m}}
	}
	r, err := x.Ops.Open(m.ID)
	if err != nil {
		return fail(err.Error())
	}
	defer r.Close()
	if err := x.Ops.VerifyPin(ctx, r); err != nil {
		return fail("exec failed: " + r.Mask(err.Error()))
	}
	name := OutName(x.Now(), flow)
	env := []string{"E2E_EXEC_OUT=" + path.Join(runfiles.ContainerE2E, runfiles.ExecDir, name)}
	res, err := r.Project.RunRunner(ctx, []string{"run", m.Suite, "--flow", flow}, env, x.Ops.Limits.Suite)
	exited := x.Now()
	if err != nil {
		return fail("exec failed: " + r.Mask(err.Error()))
	}
	fresh, ferr := x.Ops.Open(m.ID)
	if ferr == nil {
		defer fresh.Close()
		r = fresh
	}
	view := View{ExitCode: res.Code, OOMKilled: res.OOMKilled, TimedOut: res.TimedOut, Flows: []FlowView{}, Lines: []string{}}
	for _, l := range res.Lines {
		view.Lines = append(view.Lines, r.Mask(l))
	}
	result, err := runfiles.Exec(r.Root, name)
	switch {
	case runfiles.IsMissing(err):
		view.Reason = "no result file"
	case err != nil:
		view.Reason = "the result file cannot be read: " + r.Mask(err.Error())
	default:
		view.Flows = flowsOf(result.Steps, r.Mask)
		x.Ops.AppendSpans(m.ID, signal.ExecSpans(m.ID, strings.TrimSuffix(name, ".json"), *result, store.Timestamp(exited)))
	}
	return scheduler.ExecResponse{Result: view}
}

func flowsOf(steps []contract.ResultStep, mask func(string) string) []FlowView {
	var out []FlowView
	index := map[string]int{}
	for _, s := range steps {
		i, ok := index[s.Flow]
		if !ok {
			i = len(out)
			index[s.Flow] = i
			out = append(out, FlowView{ID: s.Flow})
		}
		out[i].Steps = append(out[i].Steps, StepView{ID: s.ID, Status: s.Status, Message: mask(s.Message)})
	}
	return out
}

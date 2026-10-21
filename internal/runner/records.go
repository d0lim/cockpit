package runner

import (
	"github.com/d0lim/cockpit/internal/contract"
)

const maxStepCalls = 50

func (d *Driver) resetStep() {
	d.deps.HTTP.ResetStatus()
	d.takeRecords()
}

func (d *Driver) takeRecords() ([]contract.StepCall, []contract.StepRef) {
	var refs []contract.StepRef
	if d.deps.TakeRefs != nil {
		refs = d.deps.TakeRefs()
	}
	if d.deps.HTTP == nil {
		return nil, refs
	}
	m := d.deps.Masker()
	var calls []contract.StepCall
	for _, c := range d.deps.HTTP.TakeCalls() {
		if len(calls) == maxStepCalls {
			break
		}
		call := contract.StepCall{Method: c.Method, Host: m.Mask(c.Host), Path: m.Mask(c.Path), StartedAt: iso(c.Started), DurationMs: max(c.Duration.Milliseconds(), 0)}
		if c.Status != 0 {
			status := c.Status
			call.Status = &status
		}
		calls = append(calls, call)
	}
	return calls, refs
}

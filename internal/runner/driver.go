package runner

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/capture"
	"github.com/d0lim/cockpit/internal/ports/clock"
	httpport "github.com/d0lim/cockpit/internal/ports/http"
)

const messageMax = 500

type StepRef struct {
	Flow string `json:"flow"`
	Step string `json:"step"`
}

type DriverDeps struct {
	Dir      string
	Suite    string
	Targets  []string
	Out      func(line string)
	Clock    clock.Clock
	HTTP     *httpport.Client
	Context  *capture.Context
	Ledger   contract.Ledger
	Masker   func() *mask.Masker
	Identity contract.RunnerIdentity
	ExecOut  func(target string) string
}

type Driver struct {
	deps        DriverDeps
	begun, done bool
	plans       []ExecPlan
	planIdx     int
	plan        *ExecPlan
	book        *Book
	flowIdx     int
	stepIdx     int
	flowStarted bool
	flowFailed  bool
	outcome     map[string]contract.FlowStatus
	skipAll     bool
	ok          bool
	current     *StepRef
	sentBefore  int
	started     time.Time
}

func NewDriver(deps DriverDeps) *Driver {
	if deps.Ledger.Flows == nil {
		deps.Ledger.Flows = map[string]contract.LedgerEntry{}
	}
	if deps.Ledger.SchemaVersion == 0 {
		deps.Ledger.SchemaVersion = contract.RunnerSchemaVersion
	}
	return &Driver{deps: deps, ok: true}
}

func (d *Driver) Done() bool { return d.done }

func (d *Driver) ExitCode() int {
	if d.ok {
		return 0
	}
	return 1
}

func (d *Driver) Begin(decl SuiteDecl) error {
	if d.begun {
		return errors.New("스위트는 한 번만 시작한다")
	}
	d.begun = true
	if decl.Name != d.deps.Suite {
		return fmt.Errorf("스위트 파일 %s 의 이름이 %q 이다 — 파일 이름과 같아야 한다", d.deps.Suite, decl.Name)
	}
	ordered, err := Order(decl)
	if err != nil {
		return err
	}
	if len(d.deps.Targets) == 0 {
		if err := WriteJSON(filepath.Join(d.deps.Dir, fileManifest), Manifest(decl, ordered, d.deps.Identity)); err != nil {
			return err
		}
		d.plans = []ExecPlan{{Flows: ordered}}
		return nil
	}
	plans, err := PlanExec(ordered, d.deps.Targets, d.deps.Ledger)
	if err != nil {
		return err
	}
	d.plans = plans
	return nil
}

func (d *Driver) Next() (*StepRef, error) {
	if !d.begun {
		return nil, errors.New("begin 전에 next 를 불렀다")
	}
	if d.current != nil {
		return nil, fmt.Errorf("단계 %s › %s 가 아직 끝나지 않았다", d.current.Flow, d.current.Step)
	}
	for !d.done {
		if d.plan == nil {
			if d.planIdx >= len(d.plans) {
				d.done = true
				return nil, nil
			}
			if err := d.openPlan(); err != nil {
				return nil, err
			}
			continue
		}
		if d.flowIdx >= len(d.plan.Flows) {
			if err := d.closePlan(); err != nil {
				return nil, err
			}
			continue
		}
		flow := &d.plan.Flows[d.flowIdx]
		if !d.flowStarted {
			if d.blocked(flow) {
				if err := d.skipSteps(flow, flow.Steps); err != nil {
					return nil, err
				}
				if err := d.endFlow(flow, contract.FlowStatusSkipped); err != nil {
					return nil, err
				}
				continue
			}
			if err := d.setLedger(flow.ID, contract.FlowStatusStarted); err != nil {
				return nil, err
			}
			d.flowStarted = true
		}
		if d.flowFailed || d.stepIdx >= len(flow.Steps) {
			status := contract.FlowStatusPassed
			if d.flowFailed {
				status = contract.FlowStatusFailed
			}
			if err := d.endFlow(flow, status); err != nil {
				return nil, err
			}
			continue
		}
		return d.openStep(flow)
	}
	return nil, nil
}

func (d *Driver) Finish(ref StepRef, failure *string) error {
	if d.current == nil || *d.current != ref {
		return fmt.Errorf("단계 %s › %s 는 돌고 있는 단계가 아니다", ref.Flow, ref.Step)
	}
	flow := &d.plan.Flows[d.flowIdx]
	rec := d.book.step(ref.Flow, ref.Step)
	finished := d.deps.Clock.Now()
	if failure == nil {
		rec.Status = contract.StepStatusPassed
	} else {
		rec.Status = contract.StepStatusFailed
		rec.Message = d.deps.Masker().Summarize(*failure, messageMax)
		if d.book.result.StoppedAt == nil {
			d.book.result.StoppedAt = &contract.StopPoint{Flow: ref.Flow, Step: ref.Step}
		}
	}
	duration := max(finished.Sub(d.started).Milliseconds(), 0)
	rec.DurationMs = &duration
	counter := d.deps.HTTP.Counter()
	rec.Requests = counter.Sent - d.sentBefore
	if rec.Requests > 0 && counter.LastStatus != 0 {
		status := counter.LastStatus
		rec.HTTPStatus = &status
	}
	rec.FinishedAt = iso(finished)
	if err := d.deps.Context.Save(); err != nil {
		return err
	}
	if err := d.book.flush(); err != nil {
		return err
	}
	d.deps.Out(SummaryLine(*rec))
	d.current = nil
	if rec.Status == contract.StepStatusFailed {
		if err := d.skipSteps(flow, flow.Steps[d.stepIdx+1:]); err != nil {
			return err
		}
		d.flowFailed = true
	}
	d.stepIdx++
	return nil
}

func (d *Driver) openPlan() error {
	p := d.plans[d.planIdx]
	d.planIdx++
	write := func(v any) error { return WriteJSON(filepath.Join(d.deps.Dir, fileResult), v) }
	if p.Target != "" {
		if d.deps.ExecOut == nil {
			return errors.New("exec 결과 경로가 없다")
		}
		path := d.deps.ExecOut(p.Target)
		write = func(v any) error { return WriteJSON(path, v) }
	}
	d.book = newBook(d.deps.Suite, p.Flows, iso(d.deps.Clock.Now()), write, p.Target)
	if err := d.book.flush(); err != nil {
		return err
	}
	d.plan = &p
	d.flowIdx, d.stepIdx = 0, 0
	d.flowStarted, d.flowFailed = false, false
	d.outcome = map[string]contract.FlowStatus{}
	d.skipAll = !d.ok
	return nil
}

func (d *Driver) closePlan() error {
	passed := true
	for _, o := range d.outcome {
		passed = passed && o == contract.FlowStatusPassed
	}
	if err := d.book.finish(iso(d.deps.Clock.Now())); err != nil {
		return err
	}
	d.ok = d.ok && passed
	d.plan = nil
	return nil
}

func (d *Driver) blocked(flow *FlowDecl) bool {
	if d.skipAll {
		return true
	}
	for _, r := range flow.Requires {
		if o := d.outcome[r]; o == contract.FlowStatusFailed || o == contract.FlowStatusSkipped {
			return true
		}
	}
	return false
}

func (d *Driver) endFlow(flow *FlowDecl, status contract.FlowStatus) error {
	if err := d.setLedger(flow.ID, status); err != nil {
		return err
	}
	d.outcome[flow.ID] = status
	d.flowIdx++
	d.stepIdx = 0
	d.flowStarted, d.flowFailed = false, false
	return nil
}

func (d *Driver) openStep(flow *FlowDecl) (*StepRef, error) {
	step := flow.Steps[d.stepIdx]
	rec := d.book.step(flow.ID, step.ID)
	startedAt := d.deps.Clock.Now()
	rec.Status = contract.StepStatusRunning
	rec.StartedAt = iso(startedAt)
	if err := d.book.flush(); err != nil {
		return nil, err
	}
	d.sentBefore = d.deps.HTTP.Counter().Sent
	d.deps.HTTP.ResetStatus()
	d.started = startedAt
	d.current = &StepRef{Flow: flow.ID, Step: step.ID}
	return d.current, nil
}

func (d *Driver) setLedger(id string, status contract.FlowStatus) error {
	prev, has := d.deps.Ledger.Flows[id]
	if status == contract.FlowStatusSkipped && has && prev.Status != contract.FlowStatusSkipped {
		return nil
	}
	d.deps.Ledger.Flows[id] = contract.LedgerEntry{Status: status, At: iso(d.deps.Clock.Now())}
	return WriteJSON(filepath.Join(d.deps.Dir, fileLedger), d.deps.Ledger)
}

func (d *Driver) skipSteps(flow *FlowDecl, steps []StepDecl) error {
	if len(steps) == 0 {
		return nil
	}
	for _, s := range steps {
		d.book.step(flow.ID, s.ID).Status = contract.StepStatusSkipped
	}
	if err := d.book.flush(); err != nil {
		return err
	}
	for _, s := range steps {
		d.deps.Out(SummaryLine(*d.book.step(flow.ID, s.ID)))
	}
	return nil
}

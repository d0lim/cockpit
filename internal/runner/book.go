package runner

import (
	"fmt"

	"github.com/d0lim/cockpit/internal/contract"
)

type Book struct {
	result contract.Result
	target string
	index  map[string]int
	write  func(any) error
}

func newBook(suite string, flows []FlowDecl, startedAt string, write func(any) error, target string) *Book {
	b := &Book{target: target, index: map[string]int{}, write: write}
	b.result = contract.Result{SchemaVersion: contract.RunnerSchemaVersion, Suite: suite, StartedAt: startedAt, Steps: []contract.ResultStep{}}
	for _, f := range flows {
		for _, s := range f.Steps {
			b.index[f.ID+"\x00"+s.ID] = len(b.result.Steps)
			b.result.Steps = append(b.result.Steps, contract.ResultStep{Flow: f.ID, ID: s.ID, Status: contract.StepStatusNotRun})
		}
	}
	return b
}

func (b *Book) step(flow, id string) *contract.ResultStep {
	i, ok := b.index[flow+"\x00"+id]
	if !ok {
		panic(fmt.Sprintf("결과에 없는 단계 %s › %s", flow, id))
	}
	return &b.result.Steps[i]
}

func (b *Book) payload() any {
	if b.target == "" {
		return b.result
	}
	return contract.ExecResult{Result: b.result, Target: b.target}
}

func (b *Book) flush() error {
	return b.write(b.payload())
}

func (b *Book) finish(at string) error {
	b.result.Complete = true
	b.result.FinishedAt = at
	return b.flush()
}

func SummaryLine(step contract.ResultStep) string {
	line := step.Flow + " › " + step.ID + " " + string(step.Status)
	if step.DurationMs != nil {
		line += fmt.Sprintf(" %dms", *step.DurationMs)
	}
	if step.HTTPStatus != nil {
		line += fmt.Sprintf(" [%d]", *step.HTTPStatus)
	}
	return line
}

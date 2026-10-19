package ticker

import (
	"github.com/d0lim/cockpit/internal/hub/pool"
	"github.com/d0lim/cockpit/internal/hub/store"
)

type Code string

const (
	CodeBadRequest        Code = "BAD_REQUEST"
	CodeNotFound          Code = "NOT_FOUND"
	CodeConflictState     Code = "CONFLICT_STATE"
	CodeConflictCap       Code = "CONFLICT_CAP"
	CodeConflictOwner     Code = "CONFLICT_OWNER"
	CodeConflictPort      Code = "CONFLICT_PORT"
	CodeQueueFull         Code = "QUEUE_FULL"
	CodeOpFailed          Code = "OP_FAILED"
	CodeRejectedCommit    Code = "REJECTED_COMMIT"
	CodeRejectedWorktree  Code = "REJECTED_WORKTREE"
	CodeRejectedDirty     Code = "REJECTED_DIRTY"
	CodeRejectedNoHarness Code = "REJECTED_NO_HARNESS"
	CodeRejectedHostAPI   Code = "REJECTED_HOST_API"
)

type Failure struct {
	Status     int              `json:"-"`
	Code       Code             `json:"code"`
	Reason     string           `json:"reason"`
	Run        *store.Meta      `json:"run,omitempty"`
	Candidates []pool.Candidate `json:"candidates,omitempty"`
	Files      []string         `json:"files,omitempty"`
}

func (f *Failure) Error() string { return f.Reason }

type Result struct {
	Run  store.Meta
	Fail *Failure
}

func (r Result) OK() bool { return r.Fail == nil }

func Succeeded(m store.Meta) Result { return Result{Run: m} }

func Failed(status int, code Code, reason string, run *store.Meta) Result {
	return Result{Fail: &Failure{Status: status, Code: code, Reason: reason, Run: run}}
}

func Conflict(reason string, run *store.Meta) Result {
	return Failed(409, CodeConflictState, reason, run)
}

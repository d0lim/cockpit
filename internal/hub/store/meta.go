package store

import (
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

const StateSchemaVersion = 2

type Leases map[string]int

type HubState struct {
	SchemaVersion int      `json:"schemaVersion"`
	Queue         []string `json:"queue"`
	Leases        Leases   `json:"leases"`
}

func EmptyState() HubState {
	return HubState{SchemaVersion: StateSchemaVersion, Queue: []string{}, Leases: Leases{}}
}

type Redaction struct {
	Status string   `json:"status"`
	Files  []string `json:"files"`
}

type Meta struct {
	lifecycle.State
	ID             string                  `json:"id"`
	SHA            string                  `json:"sha"`
	Repository     string                  `json:"repository,omitempty"`
	Suite          string                  `json:"suite"`
	HostAPI        int                     `json:"hostApi"`
	HarnessVersion int                     `json:"harnessVersion,omitempty"`
	CockpitVersion string                  `json:"cockpitVersion"`
	RunnerImage    *contract.RunnerImage   `json:"runnerImage,omitempty"`
	Submitter      string                  `json:"submitter"`
	Keep           bool                    `json:"keep"`
	CreatedAt      string                  `json:"createdAt"`
	StartedAt      string                  `json:"startedAt,omitempty"`
	FinishedAt     string                  `json:"finishedAt,omitempty"`
	LastUpAt       string                  `json:"lastUpAt,omitempty"`
	Slot           *int                    `json:"slot,omitempty"`
	Pgid           *int                    `json:"pgid,omitempty"`
	ImageKeys      map[string]string       `json:"imageKeys,omitempty"`
	ImageDigests   map[string]string       `json:"imageDigests,omitempty"`
	FailedStage    lifecycle.Phase         `json:"failedStage,omitempty"`
	Tail           []string                `json:"tail,omitempty"`
	Notice         string                  `json:"notice,omitempty"`
	WaitReason     lifecycle.WaitReason    `json:"waitReason,omitempty"`
	StopReason     lifecycle.StopReason    `json:"stopReason,omitempty"`
	DestroyReason  lifecycle.DestroyReason `json:"destroyReason,omitempty"`
	Recreate       bool                    `json:"recreate,omitempty"`
	Redaction      *Redaction              `json:"redaction,omitempty"`
	Stages         []StageTime             `json:"stages,omitempty"`
}

type StageTime struct {
	Stage   lifecycle.Phase     `json:"stage"`
	Start   string              `json:"start"`
	End     string              `json:"end,omitempty"`
	Outcome contract.SpanStatus `json:"outcome,omitempty"`
}

func (m *Meta) StartStage(stage lifecycle.Phase, at string) {
	m.Stages = append(m.Stages, StageTime{Stage: stage, Start: at})
}

func (m *Meta) EndStage(at string, outcome contract.SpanStatus) {
	if n := len(m.Stages); n > 0 && m.Stages[n-1].End == "" {
		m.Stages[n-1].End, m.Stages[n-1].Outcome = at, outcome
	}
}

func (m Meta) StageStart() string {
	if n := len(m.Stages); n > 0 && m.Stages[n-1].Stage == m.Phase && m.Stages[n-1].End == "" {
		return m.Stages[n-1].Start
	}
	return ""
}

func Timestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func ByCreated(a, b Meta) int {
	if a.CreatedAt != b.CreatedAt {
		if a.CreatedAt < b.CreatedAt {
			return -1
		}
		return 1
	}
	if a.ID < b.ID {
		return -1
	}
	if a.ID > b.ID {
		return 1
	}
	return 0
}

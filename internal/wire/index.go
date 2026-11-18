package wire

import (
	"time"

	"github.com/d0lim/cockpit/internal/contract"
)

type LinkKind string

const (
	LinkExact  LinkKind = "exact"
	LinkNested LinkKind = "nested"
	LinkTime   LinkKind = "time"
	LinkNone   LinkKind = "none"
)

const NoStep = -1

type Link struct {
	Kind   LinkKind `json:"kind"`
	Step   int      `json:"step"`
	Parent int64    `json:"parent,omitempty"`
}

type State = contract.WireState

const (
	StateTaken    = contract.WireStateTaken
	StateLost     = contract.WireStateLost
	StateWithheld = contract.WireStateWithheld
	StateFailed   = contract.WireStateFailed
)

type Status = contract.WireStatus

type StepRef struct {
	Flow   string              `json:"flow"`
	Step   string              `json:"step"`
	Status contract.StepStatus `json:"status,omitempty"`
}

type Head struct {
	Seq        int64     `json:"seq"`
	Caller     string    `json:"caller"`
	Target     string    `json:"target"`
	Port       int       `json:"port"`
	Method     string    `json:"method"`
	Path       string    `json:"path,omitempty"`
	Status     int       `json:"status"`
	Outcome    Outcome   `json:"outcome"`
	Withheld   bool      `json:"withheld,omitempty"`
	Started    time.Time `json:"started"`
	DurationMs int64     `json:"durationMs"`
	Link       Link      `json:"link"`
	Offset     int64     `json:"offset"`
	Length     int64     `json:"length"`
}

type Index struct {
	Status       Status                     `json:"status"`
	Steps        []StepRef                  `json:"steps"`
	Heads        []Head                     `json:"heads"`
	PublicFields []contract.WirePublicField `json:"publicFields,omitempty"`
	Routes       []contract.WireRoute       `json:"routes,omitempty"`
}

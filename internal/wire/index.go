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

type State string

const (
	StateTaken    State = "taken"
	StateLost     State = "lost"
	StateWithheld State = "withheld"
	StateFailed   State = "failed"
)

type Status struct {
	Take          string `json:"take"`
	State         State  `json:"state"`
	Detail        string `json:"detail,omitempty"`
	Calls         int    `json:"calls"`
	WithheldCalls int    `json:"withheldCalls"`
	BodiesOmitted int    `json:"bodiesOmitted"`
	CallsDropped  int    `json:"callsDropped"`
}

type StepRef struct {
	Flow string `json:"flow"`
	Step string `json:"step"`
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
}

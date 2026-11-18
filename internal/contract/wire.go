package contract

import "strings"

const (
	WireRoutesVariable = "COCKPIT_WIRE_ROUTES"
	WireValuesFile     = "wire.private.env.json"
	WireValuesLimit    = 16 << 20
	wireVariablePrefix = "COCKPIT_WIRE_"
)

type WireState string

const (
	WireStateTaken    WireState = "taken"
	WireStateLost     WireState = "lost"
	WireStateWithheld WireState = "withheld"
	WireStateFailed   WireState = "failed"
)

type WireStatus struct {
	Take          string    `json:"take"`
	State         WireState `json:"state"`
	Detail        string    `json:"detail,omitempty"`
	Calls         int       `json:"calls"`
	WithheldCalls int       `json:"withheldCalls"`
	BodiesOmitted int       `json:"bodiesOmitted"`
	CallsDropped  int       `json:"callsDropped"`
}

type WireValues struct {
	Values []string `json:"values"`
}

func WireVariable(route string) string {
	return wireVariablePrefix + strings.ToUpper(strings.ReplaceAll(route, "-", "_"))
}

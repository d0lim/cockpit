package contract

import "strings"

const (
	WireRoutesVariable = "COCKPIT_WIRE_ROUTES"
	WireValuesFile     = "wire.private.env.json"
	wireVariablePrefix = "COCKPIT_WIRE_"
)

type WireValues struct {
	Values []string `json:"values"`
}

func WireVariable(route string) string {
	return wireVariablePrefix + strings.ToUpper(strings.ReplaceAll(route, "-", "_"))
}

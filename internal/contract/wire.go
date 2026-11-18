package contract

import "strings"

const (
	WireRoutesVariable = "COCKPIT_WIRE_ROUTES"
	wireVariablePrefix = "COCKPIT_WIRE_"
)

func WireVariable(route string) string {
	return wireVariablePrefix + strings.ToUpper(strings.ReplaceAll(route, "-", "_"))
}

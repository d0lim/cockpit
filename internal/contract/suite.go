package contract

import "regexp"

var SuiteName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

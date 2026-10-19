package runenv

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
)

const Usage = "usage: cockpit run-env --project <path> [--suite <name>] [--unseal-check] [--keep]"

var suiteName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type Options struct {
	Project     string
	Suite       string
	UnsealCheck bool
	Keep        bool
}

func Parse(args []string) (Options, error) {
	var o Options
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--unseal-check":
			o.UnsealCheck = true
		case "--keep":
			o.Keep = true
		case "--project", "--suite":
			if i+1 >= len(args) || args[i+1] == "" {
				return Options{}, fmt.Errorf("%s needs a value", args[i])
			}
			if args[i] == "--project" {
				o.Project = args[i+1]
			} else {
				o.Suite = args[i+1]
			}
			i++
		default:
			return Options{}, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if o.Project == "" {
		return Options{}, errors.New("--project is required")
	}
	abs, err := filepath.Abs(o.Project)
	if err != nil {
		return Options{}, err
	}
	o.Project = abs
	if o.Suite != "" && !suiteName.MatchString(o.Suite) {
		return Options{}, fmt.Errorf("suite name %q must match %s", o.Suite, suiteName)
	}
	return o, nil
}

package headless

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/d0lim/cockpit/internal/contract"
)

const Usage = "usage: cockpit run --project <path> [--suite <name>] [--declaration <file>] [--unseal-check] [--wire] [--keep] [--out <dir>] [--archive <file>]"

var suiteName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type Options struct {
	Project     string
	Suite       string
	Declaration contract.DeclarationFile
	UnsealCheck bool
	Wire        bool
	Keep        bool
	Out         string
	Archive     string
}

func Parse(args []string) (Options, error) {
	var o Options
	var declaration string
	values := map[string]*string{"--project": &o.Project, "--suite": &o.Suite, "--declaration": &declaration, "--out": &o.Out, "--archive": &o.Archive}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--unseal-check":
			o.UnsealCheck = true
			continue
		case "--keep":
			o.Keep = true
			continue
		case "--wire":
			o.Wire = true
			continue
		}
		target, ok := values[args[i]]
		if !ok {
			return Options{}, fmt.Errorf("unknown argument %q", args[i])
		}
		if i+1 >= len(args) || args[i+1] == "" {
			return Options{}, fmt.Errorf("%s needs a value", args[i])
		}
		*target = args[i+1]
		i++
	}
	if o.Project == "" {
		return Options{}, errors.New("--project is required")
	}
	for _, p := range []*string{&o.Project, &o.Out, &o.Archive} {
		if *p == "" {
			continue
		}
		abs, err := filepath.Abs(*p)
		if err != nil {
			return Options{}, err
		}
		*p = abs
	}
	file, err := contract.ParseDeclarationFile(declaration)
	if err != nil {
		return Options{}, err
	}
	o.Declaration = file
	if o.Suite != "" && !suiteName.MatchString(o.Suite) {
		return Options{}, fmt.Errorf("suite name %q must match %s", o.Suite, suiteName)
	}
	if o.Out != "" && o.Out == o.Archive {
		return Options{}, errors.New("--out and --archive name the same path")
	}
	return o, nil
}

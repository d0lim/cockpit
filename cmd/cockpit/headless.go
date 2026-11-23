package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/headless"
	"github.com/d0lim/cockpit/internal/home"
	"github.com/d0lim/cockpit/internal/hub/cli"
	"github.com/d0lim/cockpit/internal/runnerbin"
)

const headlessSubmitter = "cli"

var headlessDocker compose.Docker = compose.CLI{}

var headlessRun = headless.Run

func runHeadless(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	opts, err := headless.Parse(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintln(stderr, headless.Usage)
		return cli.ExitRejected
	}
	dir, err := home.Dir(getenv)
	if err != nil {
		fmt.Fprintln(stderr, "cannot resolve the state directory: "+err.Error())
		return cli.ExitRejected
	}
	submitter := getenv(cli.EnvSubmitter)
	if submitter == "" {
		submitter = headlessSubmitter
	}
	pins := runnerbin.FilePins{Path: filepath.Join(dir, pinsFile)}
	return headlessRun(ctx, opts, headless.Deps{
		Docker:      headlessDocker,
		EnsureImage: func(ctx context.Context) (runnerbin.Image, error) { return ensureImage(ctx, pins) },
		Home:        dir,
		Out:         stdout,
		Submitter:   submitter,
	})
}

package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/home"
	"github.com/d0lim/cockpit/internal/runenv"
	"github.com/d0lim/cockpit/internal/runnerbin"
)

var runEnvDocker compose.Docker = compose.CLI{}

func runEnv(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	opts, err := runenv.Parse(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintln(stderr, runenv.Usage)
		return runenv.ExitRefused
	}
	dir, err := home.Dir(getenv)
	if err != nil {
		fmt.Fprintln(stderr, "cannot resolve the state directory: "+err.Error())
		return runenv.ExitRefused
	}
	pins := runnerbin.FilePins{Path: filepath.Join(dir, pinsFile)}
	return runenv.Run(ctx, opts, runenv.Deps{
		Docker:      runEnvDocker,
		EnsureImage: func(ctx context.Context) (runnerbin.Image, error) { return ensureImage(ctx, pins) },
		Home:        dir,
		Out:         stdout,
	})
}

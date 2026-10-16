package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/d0lim/cockpit/internal/home"
	"github.com/d0lim/cockpit/internal/runnerbin"
)

const pinsFile = "runner-images.json"

var ensureImage = func(ctx context.Context, pins runnerbin.Pins) (runnerbin.Image, error) {
	return runnerbin.Ensure(ctx, runnerbin.CLI{}, runnerbin.Lookup, pins)
}

func runnerImage(ctx context.Context, getenv func(string) string, stdout, stderr io.Writer) int {
	dir, err := home.Dir(getenv)
	if err != nil {
		fmt.Fprintln(stderr, "cannot resolve the state directory: "+err.Error())
		return 2
	}
	img, err := ensureImage(ctx, runnerbin.FilePins{Path: filepath.Join(dir, pinsFile)})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	fmt.Fprintln(stdout, img.Tag)
	return 0
}

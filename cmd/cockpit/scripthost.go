package main

import (
	"context"
	"io"
	"os"

	"github.com/d0lim/cockpit/internal/collect/worker"
)

var scriptHostStdin io.Reader = os.Stdin

func scriptHost(ctx context.Context, stdout, stderr io.Writer) int {
	return worker.Main(ctx, scriptHostStdin, stdout, stderr)
}

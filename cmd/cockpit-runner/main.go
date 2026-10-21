package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/d0lim/cockpit/internal/runner"
	"github.com/d0lim/cockpit/internal/version"
)

const usage = "usage: cockpit-runner run <suite> [--flow <id>]... | probe <canary-port> | edge [<port>=<host>:<port>...] | --version"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "--version":
		fmt.Fprintln(stdout, "cockpit-runner "+version.String())
		return 0
	case "run":
		return runner.Main(ctx, args[1:], getenv, runner.Deps{Stdout: stdout, Stderr: stderr})
	case "probe":
		return probeMain(ctx, args[1:], stdout, stderr)
	case "edge":
		return edgeMain(ctx, args[1:], stderr)
	}
	fmt.Fprintln(stderr, usage)
	return 2
}

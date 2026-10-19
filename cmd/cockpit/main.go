package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/d0lim/cockpit/internal/version"
)

const usage = "usage: cockpit init [--base-url <url>] [<dir>] | sdk write <dir> | runner-image | run-env --project <path> [--suite <name>] [--unseal-check] [--keep] | --version"

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
		fmt.Fprintln(stdout, "cockpit "+version.String())
		return 0
	case "init":
		return initProject(args[1:], stdout, stderr)
	case "sdk":
		if len(args) == 3 && args[1] == "write" {
			return sdkWrite(args[2], stdout, stderr)
		}
	case "run-env":
		return runEnv(ctx, args[1:], getenv, stdout, stderr)
	case "runner-image":
		if len(args) == 1 {
			return runnerImage(ctx, getenv, stdout, stderr)
		}
	}
	fmt.Fprintln(stderr, usage)
	return 2
}

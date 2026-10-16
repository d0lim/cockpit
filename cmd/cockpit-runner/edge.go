package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/d0lim/cockpit/internal/runner"
)

func edgeMain(ctx context.Context, args []string, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: cockpit-runner edge <port>=<host>:<port>...")
		return 2
	}
	routes := make([]runner.Route, 0, len(args))
	for _, spec := range args {
		r, err := runner.ParseRoute(spec)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		routes = append(routes, r)
	}
	proxies, err := runner.ListenAll(routes)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if err := runner.ServeAll(ctx, proxies); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(stderr, err)
		return 2
	}
	return 0
}

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"github.com/d0lim/cockpit/internal/runner/otlp"
)

func otlpMain(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "otlp takes exactly one listen address, such as :4318")
		return 2
	}
	_, port, err := net.SplitHostPort(args[0])
	if n, perr := strconv.Atoi(port); err != nil || perr != nil || n < 0 || n > 65535 {
		fmt.Fprintf(stderr, "otlp listen address %q must have the form [<host>]:<port>\n", args[0])
		return 2
	}
	out, err := otlp.NewWriter(stdout)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	ln, err := net.Listen("tcp", args[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	log := slog.New(slog.NewJSONHandler(stderr, nil))
	log.Info("otlp receiver listening", "addr", ln.Addr().String())
	if err := otlp.Serve(ctx, ln, &otlp.Handler{Out: out, Log: log}); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("otlp receiver stopped", "err", err)
		return 2
	}
	return 0
}

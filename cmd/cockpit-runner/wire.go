package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"strings"

	"github.com/d0lim/cockpit/internal/mask"
	recorder "github.com/d0lim/cockpit/internal/runner/wire"
)

const (
	wireUsage    = "usage: cockpit-runner wire --project <run id> [--public-field <key>=<base64url of value pattern>]... <listen port>=<service>:<port>..."
	wireCtlUsage = "usage: cockpit-runner wire-ctl arm <take> | take <take> | ping"
)

type wireArgs struct {
	project string
	fields  []mask.PublicField
	routes  []recorder.Route
}

func parseWireArgs(args []string) (wireArgs, error) {
	var parsed wireArgs
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project", "--public-field":
			if i+1 >= len(args) {
				return parsed, fmt.Errorf("%s needs a value", args[i])
			}
			i++
			if args[i-1] == "--project" {
				parsed.project = args[i]
				continue
			}
			field, err := parsePublicField(args[i])
			if err != nil {
				return parsed, err
			}
			parsed.fields = append(parsed.fields, field)
		default:
			route, err := recorder.ParseRoute(args[i])
			if err != nil {
				return parsed, err
			}
			parsed.routes = append(parsed.routes, route)
		}
	}
	if parsed.project == "" || len(parsed.routes) == 0 {
		return parsed, errors.New(wireUsage)
	}
	return parsed, nil
}

func parsePublicField(spec string) (mask.PublicField, error) {
	key, encoded, ok := strings.Cut(spec, "=")
	if !ok || key == "" {
		return mask.PublicField{}, errors.New("--public-field must have the form <key>=<base64url of value pattern>")
	}
	value, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return mask.PublicField{}, fmt.Errorf("--public-field %s holds a value that is not base64url", key)
	}
	return mask.PublicField{Key: key, Value: string(value)}, nil
}

func wireMain(ctx context.Context, args []string, getenv func(string) string, stderr io.Writer) int {
	return serveWire(ctx, args, getenv, stderr, recorder.ControlPort)
}

func serveWire(ctx context.Context, args []string, getenv func(string) string, stderr io.Writer, controlPort int) int {
	parsed, err := parseWireArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	base, err := mask.New(mask.Options{Mode: mask.Runner})
	if err != nil {
		fmt.Fprintln(stderr, "could not build the wire masker")
		return 2
	}
	masker, err := base.WithPublicFields(parsed.fields)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	log := slog.New(slog.NewJSONHandler(stderr, nil))
	rec, err := recorder.Listen(recorder.Options{Project: parsed.project, Routes: parsed.routes, Masker: masker, Log: log, ControlPort: controlPort})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(recorder.MemoryLimit)
	}
	log.Info("wire recorder listening", "routes", len(parsed.routes), "control", rec.ControlAddr())
	if err := rec.Serve(ctx); err != nil {
		log.Error("wire recorder stopped", "err", err)
		return 2
	}
	return 0
}

func wireCtlMain(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return wireCtl(ctx, recorder.ControlAddress(), args, stdout, stderr)
}

func wireCtl(ctx context.Context, addr string, args []string, stdout, stderr io.Writer) int {
	var request string
	switch {
	case len(args) == 1 && args[0] == "ping":
		request = "ping"
	case len(args) == 2 && (args[0] == "arm" || args[0] == "take") && recorder.ValidTake(args[1]):
		request = args[0] + " " + args[1]
	default:
		fmt.Fprintln(stderr, wireCtlUsage)
		return 2
	}
	if err := recorder.Control(ctx, addr, request, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

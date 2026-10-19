package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/cli"
	"github.com/d0lim/cockpit/internal/hub/pipeline"
	"github.com/d0lim/cockpit/internal/hub/pool"
	"github.com/d0lim/cockpit/internal/hub/procgroup"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/server"
	"github.com/d0lim/cockpit/internal/hub/sse"
	"github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/runnerbin"
)

const hubTick = 2 * time.Second

const pipelineJob = "pipeline"

func jobFrame(e pipeline.Event) sse.JobEvent {
	id := e.Run + "/" + pipelineJob
	switch e.Phase {
	case pipeline.PhaseStart:
		return sse.JobStarted(id, pipelineJob, e.Line)
	case pipeline.PhaseExit:
		code := 1
		if e.Code != nil {
			code = *e.Code
		}
		return sse.JobExited(id, pipelineJob, code)
	}
	return sse.JobOutput(id, pipelineJob, e.Line)
}

func pipelineBuilder(getenv func(string) string) server.Builder {
	return func(in server.BuildInput) (server.Built, error) {
		pins := runnerbin.FilePins{Path: filepath.Join(in.Home, pinsFile)}
		sched, hub, err := pipeline.New(pipeline.Options{
			Home: in.Home, Config: in.Config, HubPort: in.HubPort, Streams: in.Streams, Log: in.Log,
			DockerFor: func(h procgroup.Hooks) compose.Docker { return compose.CLI{Hooks: h} },
			Probe:     reconcile.NewCLI(),
			Ensure:    func(ctx context.Context) (runnerbin.Image, error) { return ensureImage(ctx, pins) },
			Limits:    stages.DefaultLimits(), StopOnDone: pool.StopOnDoneFrom(getenv), TickInterval: hubTick,
			Events: func(e pipeline.Event) {
				in.Bus.PublishJob(e.Run, jobFrame(e))
			},
		})
		if err != nil {
			return server.Built{}, err
		}
		return server.Built{
			Scheduler: sched, Signals: hub.Streams(), Collectors: hub.Collectors(),
			Services: hub.Services, Suites: hub.Suites, Mask: hub.Mask, Close: hub.Close,
		}, nil
	}
}

func hub(ctx context.Context, args []string, getenv func(string) string, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "usage: cockpit hub")
		return 2
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	err := server.Run(ctx, server.Options{Getenv: getenv, Build: pipelineBuilder(getenv), Log: log})
	if err != nil {
		fmt.Fprintln(stderr, "cockpit hub: "+err.Error())
		if errors.Is(err, store.ErrLegacyLayout) {
			return cli.ExitRejected
		}
		return 1
	}
	return 0
}

func runs(ctx context.Context, args []string, getenv func(string) string, cwd string, stdout, stderr io.Writer) int {
	return cli.Main(ctx, append([]string{"runs"}, args...), cli.IO{Getenv: getenv, Cwd: cwd, Stdout: stdout, Stderr: stderr})
}

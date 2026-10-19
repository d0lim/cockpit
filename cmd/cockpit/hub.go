package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/cli"
	"github.com/d0lim/cockpit/internal/hub/pool"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/server"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/version"
)

var errUnwired = errors.New("this build has no run pipeline wired into the hub")

type unwired struct{}

func (unwired) Stop(context.Context, string) ticker.Result    { return unwiredResult() }
func (unwired) Destroy(context.Context, string) ticker.Result { return unwiredResult() }
func (unwired) Resume(context.Context, string) ticker.Result  { return unwiredResult() }
func (unwired) KillGroup(int) error                           { return errUnwired }
func (unwired) RemoveOneOffs(context.Context, string) error   { return errUnwired }
func (unwired) CloseWithoutEnv(context.Context, string, bool) error {
	return errUnwired
}
func (unwired) StopQuietly(context.Context, string) {}
func (unwired) HasRunnerResult(string) bool         { return false }
func (unwired) Report(context.Context, store.Meta) (reconcile.ReportOutcome, error) {
	return reconcile.ReportOutcome{}, errUnwired
}
func (unwired) Redo(context.Context, string) error { return errUnwired }
func (unwired) Mask(_, text string) string         { return text }
func (unwired) Denied(store.Meta, bool, string) *ticker.Failure {
	return &ticker.Failure{Status: 409, Code: ticker.CodeOpFailed, Reason: errUnwired.Error()}
}
func (unwired) Run(context.Context, store.Meta, string) scheduler.ExecResponse {
	return scheduler.ExecResponse{Fail: &ticker.Failure{Status: 409, Code: ticker.CodeOpFailed, Reason: errUnwired.Error()}}
}

func unwiredResult() ticker.Result {
	return ticker.Failed(409, ticker.CodeOpFailed, errUnwired.Error(), nil)
}

func unwiredBuilder(getenv func(string) string) server.Builder {
	return func(in server.BuildInput) (server.Built, error) {
		limits := pool.Limits{PoolMax: in.Config.Limits.KeptPool, QueueMax: in.Config.Limits.QueueCap, UpCap: in.Config.Limits.RunningCap}
		d := scheduler.Deps{
			Deps: ticker.Deps{
				Store: in.Store, Docker: reconcile.NewCLI(), Env: unwired{}, Limits: limits,
				Pipeline:     func(context.Context, string) error { return errUnwired },
				Lease:        func(store.Leases, string) (int, error) { return 0, errUnwired },
				StopOnDone:   pool.StopOnDoneFrom(getenv),
				TickInterval: 2 * time.Second, Now: time.Now, Served: compose.ServedHostAPIs, CockpitVersion: version.String(), Log: in.Log,
			},
			Accept: func(context.Context, scheduler.SubmitInput) (scheduler.Accepted, error) {
				return scheduler.Accepted{}, errUnwired
			},
			Exec: unwired{},
		}
		return server.Built{Scheduler: scheduler.New(d)}, nil
	}
}

func hub(ctx context.Context, args []string, getenv func(string) string, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "usage: cockpit hub")
		return 2
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	err := server.Run(ctx, server.Options{Getenv: getenv, Build: unwiredBuilder(getenv), Log: log})
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

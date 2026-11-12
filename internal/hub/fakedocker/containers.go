package fakedocker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
)

const ContainerLogLine = "container says hello"

var OTLPContainerID = strings.Repeat("f", 64)

type OTLPLine struct {
	After time.Duration
	Line  string
}

func filterLabel(args []string, label string) string {
	for i, a := range args {
		if a == "--filter" && i+1 < len(args) {
			if v, ok := strings.CutPrefix(args[i+1], "label="+label+"="); ok {
				return v
			}
		}
	}
	return ""
}

func containerTable(args []string) string {
	project := filterLabel(args, "com.docker.compose.project")
	runID := filterLabel(args, compose.LabelRunID)
	var rows []string
	for i, svc := range []string{"app", "db", "cache"} {
		id := fmt.Sprintf("%064d", i+1)
		rows = append(rows, strings.Join([]string{id, project + "-" + svc + "-1", "running", "Up 1 minute (healthy)", project, svc, runID, ""}, "\t"))
	}
	rows = append(rows, strings.Join([]string{OTLPContainerID, project + "-otlp-1", "running", "Up 1 minute", project, compose.RoleOTLP, runID, compose.RoleOTLP}, "\t"))
	return strings.Join(rows, "\n")
}

func (f *Fake) containerLogs(ctx context.Context, cmd compose.Command) (compose.Result, error) {
	if cmd.Stdout != nil {
		stamp := time.Now().UTC().Format(time.RFC3339Nano)
		fmt.Fprintf(cmd.Stdout, "%s %s %s\n", stamp, ContainerLogLine, f.ContainerSecret)
	}
	select {
	case <-ctx.Done():
	case <-time.After(200 * time.Millisecond):
	}
	return compose.Result{}, nil
}

func (f *Fake) otlpLogs(ctx context.Context, cmd compose.Command) (compose.Result, error) {
	since := ""
	for _, a := range cmd.Args {
		if v, ok := strings.CutPrefix(a, "--since="); ok {
			since = v
		}
	}
	from, _ := time.Parse(time.RFC3339Nano, since)
	sent := map[int]bool{}
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		run, _ := f.otlpRun.Load().(string)
		if done := f.doneAt.Load(); done > 0 && f.OTLP != nil && cmd.Stdout != nil {
			for i, l := range f.OTLP(run) {
				at := time.Unix(0, done).Add(l.After).UTC()
				if !sent[i] && !time.Now().Before(at) && !at.Before(from) {
					sent[i] = true
					fmt.Fprintf(cmd.Stdout, "%s %s\n", at.Format(time.RFC3339Nano), l.Line)
				}
			}
		}
		select {
		case <-ctx.Done():
			return compose.Result{}, nil
		case <-tick.C:
		}
	}
}

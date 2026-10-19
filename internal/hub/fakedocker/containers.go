package fakedocker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
)

const ContainerLogLine = "container says hello"

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

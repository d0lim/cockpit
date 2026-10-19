package reconcile

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
)

type Runner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

type CLI struct {
	Runner      Runner
	InfoTimeout time.Duration
}

func NewCLI() CLI { return CLI{Runner: ExecRunner{}, InfoTimeout: 5 * time.Second} }

func (c CLI) ComposeLs(ctx context.Context) ([]Project, error) {
	out, err := c.Runner.Run(ctx, "compose", "ls", "-a", "--format", "json")
	if err != nil {
		return nil, err
	}
	raws, err := compose.ParseJSONRows([]byte(out))
	if err != nil {
		return nil, fmt.Errorf("docker compose ls returned unreadable output: %w", err)
	}
	type row struct {
		Name   string `json:"Name"`
		Status string `json:"Status"`
	}
	rows := make([]row, 0, len(raws))
	for _, raw := range raws {
		var r row
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, fmt.Errorf("docker compose ls returned unreadable output: %w", err)
		}
		rows = append(rows, r)
	}
	projects := make([]Project, 0, len(rows))
	for _, r := range rows {
		projects = append(projects, Project{Name: r.Name, Status: r.Status})
	}
	return projects, nil
}

func (c CLI) Info(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, c.InfoTimeout)
	defer cancel()
	_, err := c.Runner.Run(ctx, "info", "--format", "{{.ServerVersion}}")
	return err == nil
}

func (c CLI) count(ctx context.Context, args ...string) (int, error) {
	out, err := c.Runner.Run(ctx, args...)
	if err != nil {
		return 0, err
	}
	return len(strings.Fields(out)), nil
}

func (c CLI) Labelled(ctx context.Context, project string) (Labelled, error) {
	own := "label=" + LabelRunID + "=" + project
	containers, err := c.count(ctx, "ps", "-aq", "--filter", own)
	if err != nil {
		return Labelled{}, err
	}
	networks, err := c.count(ctx, "network", "ls", "-q", "--filter", own)
	if err != nil {
		return Labelled{}, err
	}
	volumes, err := c.count(ctx, "volume", "ls", "-q", "--filter", "label="+LabelComposeProject+"="+project)
	if err != nil {
		return Labelled{}, err
	}
	return Labelled{Containers: containers, Networks: networks, Volumes: volumes}, nil
}

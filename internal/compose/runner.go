package compose

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var containerID = regexp.MustCompile(`^[0-9a-f]{12,64}$`)

type RunnerResult struct {
	ContainerID string
	Code        *int
	OOMKilled   bool
	TimedOut    bool
	Aborted     bool
	Lines       []string
}

func (p Project) docker(ctx context.Context, step string, args ...string) (Result, error) {
	return p.run(ctx, step, args)
}

func (p Project) masked(out []byte) []string {
	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, p.Mask(l))
		}
	}
	return lines
}

func (p Project) RunRunner(ctx context.Context, args, env []string, limit time.Duration) (RunnerResult, error) {
	sub := append([]string{"--profile", RoleRunner, "run", "-d", "-T", "--no-deps", RoleRunner}, args...)
	started, err := p.Docker.Run(ctx, Command{Args: append(slices.Clone(p.Args), sub...), Env: env})
	lines := strings.Split(strings.TrimSpace(string(started.Stdout)), "\n")
	id := strings.TrimSpace(lines[len(lines)-1])
	if err != nil || started.Code != 0 || !containerID.MatchString(id) {
		p.RemoveOneOffs(context.WithoutCancel(ctx), RoleRunner)
		if ctx.Err() != nil {
			return RunnerResult{Aborted: true}, nil
		}
		return RunnerResult{}, &Failed{Step: "runner start", Code: started.Code, Tail: p.Mask(tail(string(started.Stderr), failureTail))}
	}
	waitCtx, cancel := context.WithTimeout(ctx, limit)
	waited, werr := p.Docker.Run(waitCtx, Command{Args: []string{"wait", id}})
	timedOut := waitCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil
	cancel()
	cleanup := context.WithoutCancel(ctx)
	logs, _ := p.Docker.Run(cleanup, Command{Args: []string{"logs", id}})
	r := RunnerResult{ContainerID: id, Lines: append(p.masked(logs.Stdout), p.masked(logs.Stderr)...)}
	if timedOut || ctx.Err() != nil {
		r.TimedOut, r.Aborted = timedOut, !timedOut
		_, err := p.docker(cleanup, "docker rm", "rm", "-f", id)
		return r, err
	}
	code, cerr := strconv.Atoi(strings.TrimSpace(string(waited.Stdout)))
	if werr != nil || waited.Code != 0 || cerr != nil {
		p.docker(cleanup, "docker rm", "rm", "-f", id)
		return r, fmt.Errorf("docker wait did not return an exit code (exit code %d)", waited.Code)
	}
	r.Code = &code
	oom, _ := p.Docker.Run(cleanup, Command{Args: []string{"inspect", "--format", "{{.State.OOMKilled}}", id}})
	r.OOMKilled = strings.TrimSpace(string(oom.Stdout)) == "true"
	_, err = p.docker(cleanup, "docker rm", "rm", id)
	return r, err
}

func (p Project) RemoveOneOffs(ctx context.Context, service string) (int, error) {
	args := []string{"ps", "-aq", "--filter", "label=com.docker.compose.project=" + p.Name, "--filter", "label=com.docker.compose.oneoff=True"}
	if service != "" {
		args = append(args, "--filter", "label=com.docker.compose.service="+service)
	}
	listed, err := p.docker(ctx, "docker ps", args...)
	if err != nil {
		return 0, err
	}
	ids := strings.Fields(string(listed.Stdout))
	if len(ids) > 0 {
		if _, err := p.docker(ctx, "docker rm", append([]string{"rm", "-f"}, ids...)...); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

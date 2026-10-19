package stages

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/seal"
)

func OpenProject(runID string, l compose.Layout, decl contract.Declaration, d compose.Docker, mask func(string) string, log func(string)) (compose.Project, error) {
	args, err := compose.ComposeArgs(runID, l, decl)
	if err != nil {
		return compose.Project{}, err
	}
	return compose.Project{Docker: d, Name: runID, Args: args, Mask: mask, Log: log}, nil
}

func ProbeRunner(p compose.Project) seal.RunProbe {
	return func(ctx context.Context, service string) (seal.ProbeRun, error) {
		res, err := p.RunService(ctx, compose.RoleProbe, service)
		return seal.ProbeRun{Code: res.Code, Lines: strings.Split(string(res.Stdout), "\n")}, err
	}
}

type CanaryListener func(port int) (*seal.Canary, error)

func Probe(ctx context.Context, p compose.Project, base int, listen CanaryListener) ([]seal.Result, int, error) {
	if listen == nil {
		listen = seal.ListenCanary
	}
	canary, err := listen(compose.CanaryPort(base))
	if err != nil {
		return nil, 0, err
	}
	defer canary.Close()
	results := seal.Probes(ctx, ProbeRunner(p), canary, compose.CanaryPort(base))
	return results, canary.Accepted(), nil
}

func (e *Env) SealProbes(ctx context.Context) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, e.Limits.Call)
	defer cancel()
	results, accepted, err := Probe(ctx, e.Project, e.Base, e.ListenCanary)
	if err != nil {
		return 0, err
	}
	e.Probes = results
	if err := e.WriteSeal(); err != nil {
		return accepted, err
	}
	return accepted, sealFailure("the seal", append(append([]seal.Result{}, e.Static...), results...))
}

func Readiness(rows []compose.PsRow, expected []string, oneShot map[string]bool) (bool, string, error) {
	var waiting []string
	for _, name := range expected {
		var row *compose.PsRow
		for i := range rows {
			if rows[i].Service == name && !rows[i].OneOff() {
				row = &rows[i]
			}
		}
		switch {
		case row == nil:
			waiting = append(waiting, name+" (no container)")
		case oneShot[name] && row.State == "exited" && row.ExitCode == 0:
		case oneShot[name] && (row.State == "exited" || row.State == "dead"):
			return false, "", fmt.Errorf("one-shot service %s failed (%s, exit code %d)", name, row.State, row.ExitCode)
		case row.State == "exited" || row.State == "dead":
			return false, "", fmt.Errorf("%s stopped (%s, exit code %d)", name, row.State, row.ExitCode)
		case row.State != "running" || (row.Health != "" && row.Health != "healthy"):
			waiting = append(waiting, fmt.Sprintf("%s (%s %s)", name, row.State, row.Health))
		}
	}
	return len(waiting) == 0, strings.Join(waiting, ", "), nil
}

func AwaitBoot(ctx context.Context, p compose.Project, expected []string, oneShot map[string]bool, limit, poll time.Duration) error {
	deadline := time.Now().Add(limit)
	for {
		rows, err := p.Ps(ctx)
		if err != nil {
			return err
		}
		ready, waiting, err := Readiness(rows, expected, oneShot)
		if err != nil {
			return &Failed{Message: "boot failed: " + err.Error()}
		}
		if ready {
			return nil
		}
		if time.Now().After(deadline) {
			return &Failed{Message: fmt.Sprintf("boot failed: not ready within %s: %s", limit, waiting)}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
	}
}

func (e *Env) Boot(ctx context.Context) error {
	if err := e.Project.Up(ctx); err != nil {
		return err
	}
	config := e.Config
	if config == nil {
		var err error
		if config, err = e.Project.Config(ctx); err != nil {
			return err
		}
	}
	return AwaitBoot(ctx, e.Project, e.Loaded.Declaration.Project.ExpectedServices, compose.OneShots(config), e.Limits.Boot, e.Limits.Poll)
}

func (e *Env) RunSuite(ctx context.Context, suite string) (compose.RunnerResult, error) {
	return e.Project.RunRunner(ctx, []string{"run", suite}, nil, e.Limits.Suite)
}

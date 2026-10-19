package runenv

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
)

const (
	bootLimit    = 5 * time.Minute
	bootPoll     = 2 * time.Second
	runnerLimit  = 20 * time.Minute
	cleanupLimit = 2 * time.Minute
	resultFile   = "work/e2e/result.json"
)

func readiness(rows []compose.PsRow, expected []string) (bool, string, error) {
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
		case row.State == "exited" || row.State == "dead":
			return false, "", fmt.Errorf("%s stopped (%s, exit code %d)", name, row.State, row.ExitCode)
		case row.State != "running" || (row.Health != "" && row.Health != "healthy"):
			waiting = append(waiting, fmt.Sprintf("%s (%s %s)", name, row.State, row.Health))
		}
	}
	return len(waiting) == 0, strings.Join(waiting, ", "), nil
}

func (r *run) boot(ctx context.Context) (string, error) {
	if err := r.project.Up(ctx); err != nil {
		return "", err
	}
	expected := r.loaded.Declaration.Project.ExpectedServices
	deadline := time.Now().Add(bootLimit)
	for {
		rows, err := r.project.Ps(ctx)
		if err != nil {
			return "", err
		}
		ready, waiting, err := readiness(rows, expected)
		if err != nil {
			return "", err
		}
		if ready {
			return fmt.Sprintf("%d expected services running and healthy", len(expected)), nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("not ready within %s: %s", bootLimit, waiting)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(bootPoll):
		}
	}
}

func (r *run) runner(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, runnerLimit)
	defer cancel()
	res, err := r.project.RunService(ctx, compose.RoleRunner, compose.RoleRunner, "run", r.opts.Suite)
	for _, out := range [][]byte{res.Stdout, res.Stderr} {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.TrimSpace(line) != "" {
				r.logLine(line)
			}
		}
	}
	if err != nil {
		return "", err
	}
	if err := r.refreshMasker(); err != nil {
		return "", err
	}
	detail := fmt.Sprintf("suite %s exit code %d; %s", r.opts.Suite, res.Code, r.stepCounts())
	if res.Code != 0 {
		return detail, fmt.Errorf("the runner did not pass (see %s)", logFile)
	}
	return detail, nil
}

func (r *run) stepCounts() string {
	f, err := r.root.Open(resultFile)
	if err != nil {
		return "no result.json"
	}
	defer f.Close()
	var result contract.Result
	if err := contract.Decode(f, &result); err != nil {
		return "unreadable result.json"
	}
	counts := map[contract.StepStatus]int{}
	for _, s := range result.Steps {
		counts[s.Status]++
	}
	return fmt.Sprintf("%d steps: %d passed, %d failed, %d skipped", len(result.Steps), counts[contract.StepStatusPassed], counts[contract.StepStatusFailed], counts[contract.StepStatusSkipped])
}

func (r *run) cleanup(ctx context.Context) {
	if r.log != nil {
		defer r.log.Close()
	}
	if r.opts.Keep {
		if r.root != nil {
			r.st.say("kept: run directory %s; remove with: docker compose -p %s --profile '*' down -v --remove-orphans && rm -rf %s", r.layout.Root, r.runID, r.layout.Root)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupLimit)
	defer cancel()
	r.st.run("cleanup", func() (string, error) {
		var notes []string
		if r.project.Docker != nil {
			if err := r.project.Down(ctx); err != nil {
				return "", err
			}
			left := 0
			for _, name := range []string{r.runID, r.runID + "-unsealed"} {
				p := r.project
				p.Name = name
				n, err := p.Leftovers(ctx)
				if err != nil {
					return "", err
				}
				left += n
			}
			notes = append(notes, fmt.Sprintf("project %s down -v, %d labeled resources left", r.runID, left))
			if left > 0 {
				return strings.Join(notes, "; "), fmt.Errorf("resources remain")
			}
		}
		if r.root != nil {
			r.root.Close()
			if err := os.RemoveAll(r.layout.Root); err != nil {
				return strings.Join(notes, "; "), err
			}
			notes = append(notes, "run directory removed")
		}
		return strings.Join(notes, "; "), nil
	})
}

package runenv

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
)

const (
	cleanupLimit = 2 * time.Minute
	resultFile   = "work/e2e/result.json"
)

func (r *run) boot(ctx context.Context) (string, error) {
	if err := r.env.Boot(ctx); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d expected services running and healthy", len(r.loaded.Declaration.Project.ExpectedServices)), nil
}

func (r *run) runner(ctx context.Context) (string, error) {
	res, err := r.env.RunSuite(ctx, r.opts.Suite)
	if err := r.env.RefreshMasker(); err != nil {
		return "", err
	}
	for _, line := range res.Lines {
		r.env.Log(r.mask(line))
	}
	if err != nil {
		return "", err
	}
	code := -1
	if res.Code != nil {
		code = *res.Code
	}
	detail := fmt.Sprintf("suite %s exit code %d; %s", r.opts.Suite, code, r.stepCounts())
	if code != 0 || res.OOMKilled || res.TimedOut || res.Aborted {
		return detail, fmt.Errorf("the runner did not pass (see %s)", logFile)
	}
	return detail, nil
}

func (r *run) stepCounts() string {
	f, err := r.env.Root.Open(resultFile)
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
		if r.env.Root != nil {
			r.st.say("kept: run directory %s; remove with: docker compose -p %s --profile '*' down -v --remove-orphans && rm -rf %s", r.env.Layout.Root, r.runID, r.env.Layout.Root)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupLimit)
	defer cancel()
	r.st.run("cleanup", func() (string, error) {
		var notes []string
		if r.env.Project.Docker != nil {
			if err := r.env.Project.Down(ctx); err != nil {
				return "", err
			}
			left := 0
			for _, name := range []string{r.runID, r.runID + "-unsealed"} {
				p := r.env.Project
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
		if r.env.Root != nil {
			r.env.Root.Close()
			if err := os.RemoveAll(r.env.Layout.Root); err != nil {
				return strings.Join(notes, "; "), err
			}
			notes = append(notes, "run directory removed")
		}
		return strings.Join(notes, "; "), nil
	})
}

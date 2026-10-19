package runenv

import (
	"context"
	"fmt"
	"slices"
	"strings"

	hubstages "github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/seal"
)

const (
	unsealFile     = "unsealed.yaml"
	unsealOverride = "networks:\n  default:\n    internal: false\n"
)

func (r *run) build(ctx context.Context) (string, error) {
	if err := r.env.Build(ctx); err != nil {
		return "", err
	}
	return "builder and compose build finished", nil
}

func (r *run) sealProbes(ctx context.Context) (string, error) {
	accepted, err := r.env.SealProbes(ctx)
	results := r.env.Probes
	verdict := seal.Verdict(append(slices.Clone(r.env.Static), results...))
	detail := fmt.Sprintf("%s: probe %d/4 blocked, control %d/4 reached, canary accepted %d; %s written",
		verdict, count(results, "probe."), count(results, "control."), accepted, seal.FileName)
	return detail, err
}

func count(results []seal.Result, prefix string) int {
	n := 0
	for _, x := range results {
		if strings.HasPrefix(x.Name, prefix) && x.OK {
			n++
		}
	}
	return n
}

func failures(results []seal.Result) string {
	var out []string
	for _, x := range results {
		if !x.OK {
			out = append(out, x.Name+": "+x.Detail)
		}
	}
	return strings.Join(out, " | ")
}

func Bites(results []seal.Result) bool {
	red := 0
	for _, x := range results {
		if strings.HasPrefix(x.Name, "probe.") && !x.OK && !x.Inconclusive {
			red++
		}
	}
	return red == 4
}

func (r *run) unsealCheck(ctx context.Context) (string, error) {
	f, err := r.env.Root.Create(unsealFile)
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(unsealOverride)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	unsealed := r.env.Project
	unsealed.Name = r.runID + "-unsealed"
	unsealed.Args = append(slices.Clone(r.env.Project.Args), "-f", r.env.Layout.Root+"/"+unsealFile)
	unsealed.Args[slices.Index(unsealed.Args, "-p")+1] = unsealed.Name
	results, accepted, err := hubstages.Probe(ctx, unsealed, r.base)
	downErr := unsealed.Down(context.WithoutCancel(ctx))
	if err != nil {
		return "", err
	}
	detail := fmt.Sprintf("%s with default internal: false: probe %d/4 reached, canary accepted %d, project removed", unsealed.Name, 4-count(results, "probe."), accepted)
	if downErr != nil {
		return detail, downErr
	}
	if !Bites(results) {
		return detail, fmt.Errorf("the probes did not turn red without the seal: %s", failures(results))
	}
	return detail + "; the probes turn red", nil
}

package runenv

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/seal"
)

const (
	unsealFile     = "unsealed.yaml"
	unsealOverride = "networks:\n  default:\n    internal: false\n"
)

func (r *run) build(ctx context.Context) (string, error) {
	if err := r.project.Build(ctx, r.loaded.Declaration.Project.Builder); err != nil {
		return "", err
	}
	return "builder and compose build finished", nil
}

func probeRunner(p compose.Project) seal.RunProbe {
	return func(ctx context.Context, service string) (seal.ProbeRun, error) {
		res, err := p.RunService(ctx, compose.RoleProbe, service)
		return seal.ProbeRun{Code: res.Code, Lines: strings.Split(string(res.Stdout), "\n")}, err
	}
}

func (r *run) probe(ctx context.Context, p compose.Project) ([]seal.Result, int, error) {
	canary, err := seal.ListenCanary(compose.CanaryPort(r.base))
	if err != nil {
		return nil, 0, err
	}
	defer canary.Close()
	results := seal.Probes(ctx, probeRunner(p), canary, compose.CanaryPort(r.base))
	return results, canary.Accepted(), nil
}

func (r *run) sealProbes(ctx context.Context) (string, error) {
	results, accepted, err := r.probe(ctx, r.project)
	if err != nil {
		return "", err
	}
	r.probes = results
	if err := r.writeSeal(); err != nil {
		return "", err
	}
	all := append(slices.Clone(r.static), r.probes...)
	verdict := seal.Verdict(all)
	detail := fmt.Sprintf("%s: probe %d/4 blocked, control %d/4 reached, canary accepted %d; %s written",
		verdict, count(results, "probe."), count(results, "control."), accepted, seal.FileName)
	if verdict != contract.SealPassed {
		return detail, fmt.Errorf("seal %s: %s", verdict, failures(results))
	}
	return detail, nil
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
	if err := writeFile(r, unsealFile, unsealOverride); err != nil {
		return "", err
	}
	unsealed := r.project
	unsealed.Name = r.runID + "-unsealed"
	unsealed.Args = append(slices.Clone(r.project.Args), "-f", r.layout.Root+"/"+unsealFile)
	unsealed.Args[slices.Index(unsealed.Args, "-p")+1] = unsealed.Name
	results, accepted, err := r.probe(ctx, unsealed)
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

func writeFile(r *run, name, data string) error {
	f, err := r.root.Create(name)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

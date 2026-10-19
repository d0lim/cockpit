package compose

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

type EnvMissing struct {
	Containers []string
	Networks   []string
}

func (e *EnvMissing) Error() string {
	return fmt.Sprintf("the environment is gone: no containers for %s, no networks %s", orNone(e.Containers), orNone(e.Networks))
}

func orNone(s []string) string {
	if len(s) == 0 {
		return "-"
	}
	return strings.Join(s, ",")
}

type StartStep struct {
	Service string
	Healthy []string
}

func dependsOn(svc map[string]any) map[string]string {
	out := map[string]string{}
	deps, _ := svc["depends_on"].(map[string]any)
	for name, raw := range deps {
		cond, _ := raw.(map[string]any)["condition"].(string)
		if cond == "" {
			cond = "service_started"
		}
		out[name] = cond
	}
	return out
}

func ResidentOrder(config map[string]any) ([]StartStep, error) {
	services, _ := config["services"].(map[string]any)
	oneShot := map[string]bool{}
	for name, raw := range services {
		svc, _ := raw.(map[string]any)
		if svc["restart"] == "no" || len(asStrings(svc["profiles"])) > 0 {
			oneShot[name] = true
		}
	}
	var waits func(name string, seen map[string]bool) []string
	waits = func(name string, seen map[string]bool) []string {
		svc, _ := services[name].(map[string]any)
		var healthy []string
		for dep, cond := range dependsOn(svc) {
			switch {
			case oneShot[dep] && !seen[dep]:
				seen[dep] = true
				healthy = append(healthy, waits(dep, seen)...)
			case cond == "service_healthy":
				healthy = append(healthy, dep)
			}
		}
		sort.Strings(healthy)
		return healthy
	}
	pending := map[string][]string{}
	for name := range services {
		if !oneShot[name] {
			pending[name] = waits(name, map[string]bool{})
		}
	}
	var order []StartStep
	for len(pending) > 0 {
		var ready []string
		for name := range pending {
			ok := true
			for _, dep := range effective(services, oneShot, name, map[string]bool{}) {
				if _, waiting := pending[dep]; waiting {
					ok = false
				}
			}
			if ok {
				ready = append(ready, name)
			}
		}
		if len(ready) == 0 {
			return nil, fmt.Errorf("dependencies form a cycle among %s", strings.Join(sortedKeys(pending), ","))
		}
		sort.Strings(ready)
		order = append(order, StartStep{Service: ready[0], Healthy: pending[ready[0]]})
		delete(pending, ready[0])
	}
	return order, nil
}

func effective(services map[string]any, oneShot map[string]bool, name string, seen map[string]bool) []string {
	svc, _ := services[name].(map[string]any)
	var out []string
	for dep := range dependsOn(svc) {
		if seen[dep] {
			continue
		}
		seen[dep] = true
		if oneShot[dep] {
			out = append(out, effective(services, oneShot, dep, seen)...)
		} else {
			out = append(out, dep)
		}
	}
	return out
}

func asStrings(v any) []string {
	var out []string
	if l, ok := v.([]any); ok {
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func (p Project) Stop(ctx context.Context) error {
	_, err := p.ByName(ctx, "compose stop", "stop")
	return err
}

func (p Project) UpRecreate(ctx context.Context) error {
	_, err := p.Compose(ctx, "compose up", "up", "-d", "--no-build", "--force-recreate")
	return err
}

func (p Project) Start(ctx context.Context, limit time.Duration, poll time.Duration) error {
	deadline := time.Now().Add(limit)
	res, err := p.Compose(ctx, "compose config", "config", "--format", "json")
	if err != nil {
		return err
	}
	config, err := decodeConfig(res.Stdout)
	if err != nil {
		return err
	}
	steps, err := ResidentOrder(config)
	if err != nil {
		return err
	}
	rows, err := p.Ps(ctx)
	if err != nil {
		return err
	}
	idsOf := func(rows []PsRow, service string) []string {
		var ids []string
		for _, r := range rows {
			if r.Service == service && !r.OneOff() {
				ids = append(ids, r.ID)
			}
		}
		return ids
	}
	missing := &EnvMissing{}
	for _, s := range steps {
		if len(idsOf(rows, s.Service)) == 0 {
			missing.Containers = append(missing.Containers, s.Service)
		}
	}
	networks, _ := config["networks"].(map[string]any)
	for _, key := range sortedKeys(networks) {
		n, _ := networks[key].(map[string]any)
		name, _ := n["name"].(string)
		if name == "" || n["external"] == true {
			continue
		}
		if r, err := p.Docker.Run(ctx, Command{Args: []string{"network", "inspect", name}}); err != nil || r.Code != 0 {
			missing.Networks = append(missing.Networks, name)
		}
	}
	if len(missing.Containers) > 0 || len(missing.Networks) > 0 {
		return missing
	}
	for _, s := range steps {
		for _, dep := range s.Healthy {
			if err := p.awaitHealthy(ctx, dep, deadline, poll); err != nil {
				return err
			}
		}
		if rows, err = p.Ps(ctx); err != nil {
			return err
		}
		if _, err := p.docker(ctx, "docker start", append([]string{"start"}, idsOf(rows, s.Service)...)...); err != nil {
			return err
		}
	}
	return nil
}

var ErrNotHealthy = errors.New("a dependency did not become healthy")

func (p Project) awaitHealthy(ctx context.Context, service string, deadline time.Time, poll time.Duration) error {
	for {
		rows, err := p.Ps(ctx)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(rows, func(r PsRow) bool { return r.Service == service && !r.OneOff() })
		switch {
		case i >= 0 && rows[i].Health == "healthy":
			return nil
		case i < 0 || rows[i].State == "exited" || rows[i].State == "dead":
			return fmt.Errorf("%w: %s stopped", ErrNotHealthy, service)
		case time.Now().After(deadline):
			return fmt.Errorf("%w: %s within the limit", ErrNotHealthy, service)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
	}
}

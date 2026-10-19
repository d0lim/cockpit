package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type RenderedService struct {
	Image    string                     `json:"image"`
	Labels   map[string]string          `json:"labels"`
	Networks map[string]json.RawMessage `json:"networks"`
	Profiles []string                   `json:"profiles"`
	Ports    []json.RawMessage          `json:"ports"`
}

type Rendered struct {
	Services map[string]RenderedService `json:"services"`
	Networks map[string]struct {
		Internal bool `json:"internal"`
	} `json:"networks"`
}

func Render(ctx context.Context, d Docker, args []string, maskText func(string) string) (Rendered, error) {
	full := append(append([]string{}, args...), "--profile", "*", "config", "--format", "json")
	res, err := d.Run(ctx, Command{Args: full})
	if err != nil {
		return Rendered{}, fmt.Errorf("compose config: %w", err)
	}
	if res.Code != 0 {
		return Rendered{}, fmt.Errorf("compose config failed (exit code %d): %s", res.Code, maskText(tail(string(res.Stderr), envStderrTail)))
	}
	var r Rendered
	if err := json.Unmarshal(res.Stdout, &r); err != nil {
		return Rendered{}, fmt.Errorf("compose config did not print JSON")
	}
	return r, nil
}

func requiredRoles(builder string) []string {
	roles := []string{RoleEdge, RoleRunner, RoleProbe, RoleProbeControl}
	if builder != "" {
		roles = append(roles, RoleBuilder)
	}
	return roles
}

func AssertRoles(r Rendered, runID, builder string) error {
	found := map[string][]string{}
	for name, svc := range r.Services {
		if svc.Labels[LabelManaged] != "true" || svc.Labels[LabelRunID] != runID {
			return fmt.Errorf("service %s does not carry the labels of run %s", name, runID)
		}
		if role := svc.Labels[LabelRole]; role != "" {
			found[role] = append(found[role], name)
		}
	}
	var missing []string
	for _, role := range requiredRoles(builder) {
		if len(found[role]) != 1 {
			missing = append(missing, fmt.Sprintf("%s (found %d)", role, len(found[role])))
		}
	}
	if b := r.Services[builder]; builder != "" && b.Labels[LabelRole] != RoleBuilder {
		missing = append(missing, "builder service "+builder)
	}
	if len(missing) > 0 {
		return fmt.Errorf("rendered config lacks exactly one service for each role: %s", strings.Join(missing, ", "))
	}
	return nil
}

package dockerport

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/ports"
)

const (
	labelProject = "com.docker.compose.project"
	labelService = "com.docker.compose.service"
)

var serviceName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)

var psFormat = strings.Join([]string{
	"{{.ID}}", "{{.Names}}", "{{.State}}", "{{.Status}}",
	`{{.Label "` + labelProject + `"}}`, `{{.Label "` + labelService + `"}}`,
	`{{.Label "` + compose.LabelRunID + `"}}`, `{{.Label "` + compose.LabelRole + `"}}`,
}, "\t")

type ServiceState struct {
	Service string `json:"service"`
	State   string `json:"state"`
	Health  string `json:"health,omitempty"`
}

type container struct {
	id, name, state, status, service, role string
}

func (p *Port) list(ctx context.Context) ([]container, error) {
	cctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	res, err := p.docker.Run(cctx, compose.Command{Args: []string{
		"ps", "-a", "--no-trunc",
		"--filter", "label=" + labelProject + "=" + p.project,
		"--filter", "label=" + compose.LabelRunID + "=" + p.runID,
		"--format", psFormat,
	}})
	if err != nil || res.Code != 0 {
		return nil, p.failure(cctx, err, res, p.timeout, "docker ps")
	}
	var out []container
	for _, line := range strings.Split(strings.TrimSpace(string(res.Stdout)), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 8 || f[4] != p.project || f[6] != p.runID {
			continue
		}
		out = append(out, container{id: f[0], name: f[1], state: f[2], status: f[3], service: f[5], role: f[7]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

func (p *Port) target(c container) bool {
	return c.role == "" && !p.harness[c.service]
}

func (p *Port) resolve(ctx context.Context, service string) (string, error) {
	if !serviceName.MatchString(service) {
		return "", ports.BadArgs(p.mask, "not a compose service name")
	}
	if p.harness[service] {
		return "", ports.Denied(p.mask, fmt.Sprintf("service %q belongs to the cockpit harness and is not a target", service))
	}
	cs, err := p.list(ctx)
	if err != nil {
		return "", err
	}
	for _, c := range cs {
		if c.service != service {
			continue
		}
		if !p.target(c) {
			return "", ports.Denied(p.mask, fmt.Sprintf("service %q belongs to the cockpit harness and is not a target", service))
		}
		return c.id, nil
	}
	return "", ports.Denied(p.mask, fmt.Sprintf("service %q has no container in this run", service))
}

func (p *Port) PS(ctx context.Context) ([]ServiceState, error) {
	cs, err := p.list(ctx)
	if err != nil {
		return nil, err
	}
	out := []ServiceState{}
	for _, c := range cs {
		if p.target(c) {
			out = append(out, ServiceState{Service: c.service, State: c.state, Health: health(c.status)})
		}
	}
	return out, nil
}

func health(status string) string {
	switch {
	case strings.Contains(status, "(healthy)"):
		return "healthy"
	case strings.Contains(status, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(status, "(health: starting)"):
		return "starting"
	}
	return ""
}

package compose

import (
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
)

const (
	maxWireRoutes    = 64
	firstWirePort    = 41001
	runnerBinaryPath = "/cockpit-runner"
)

type WireRoute struct {
	Name    string
	Service string
	Port    int
	Listen  int
}

func (r WireRoute) Upstream() string { return r.Service + ":" + strconv.Itoa(r.Port) }

func (r WireRoute) Proxy() string { return RoleWire + ":" + strconv.Itoa(r.Listen) }

func (r WireRoute) Variable() string { return contract.WireVariable(r.Name) }

func WireRoutes(d contract.Declaration) []WireRoute {
	routes := make([]WireRoute, 0, len(d.Wire.Routes))
	for i, r := range d.Wire.Routes {
		routes = append(routes, WireRoute{Name: string(r.Name), Service: r.Service, Port: r.Port, Listen: firstWirePort + i})
	}
	return routes
}

func wireCommand(runID string, d contract.Declaration) []string {
	command := []string{"wire", "--project", runID}
	for _, f := range d.Wire.PublicFields {
		command = append(command, "--public-field", f.Key+"="+base64.RawURLEncoding.EncodeToString([]byte(f.Value)))
	}
	for _, r := range WireRoutes(d) {
		command = append(command, strconv.Itoa(r.Listen)+"="+r.Upstream())
	}
	return command
}

func WireRunnerRoutes(routes []WireRoute) string {
	parts := make([]string, 0, len(routes))
	for _, r := range routes {
		parts = append(parts, r.Name+"="+r.Upstream()+"="+r.Proxy())
	}
	return strings.Join(parts, ",")
}

func addWireRecorder(in HarnessInput, services map[string]any, inner []string, recorder map[string]any) error {
	decl := in.Loaded.Declaration
	routes := WireRoutes(decl)
	if len(routes) == 0 {
		return errors.New("a recording run needs at least one wire route")
	}
	recorder["command"] = wireCommand(in.RunID, decl)
	recorder["networks"] = inner
	recorder["healthcheck"] = map[string]any{
		"test":     []string{"CMD", runnerBinaryPath, "wire-ctl", "ping"},
		"interval": "1s",
		"timeout":  "1s",
		"retries":  30,
	}
	services[RoleWire] = recorder
	for _, name := range in.Loaded.Model.ServiceNames() {
		if name != decl.Project.Builder {
			services[name].(map[string]any)["depends_on"] = map[string]any{RoleWire: map[string]any{"condition": "service_healthy"}}
		}
	}
	runner := services[RoleRunner].(map[string]any)
	runner["environment"].(map[string]string)[contract.WireRoutesVariable] = WireRunnerRoutes(routes)
	return nil
}

func checkPublicFields(d contract.Declaration) error {
	for i, f := range d.Wire.PublicFields {
		if err := mask.CheckPublicField(mask.PublicField(f)); err != nil {
			return reject(RejectSecrets, fmt.Sprintf("wire.publicFields[%d]", i), "%v", err)
		}
	}
	return nil
}

func (m Model) checkWireRoutes(d contract.Declaration) error {
	routes := d.Wire.Routes
	if len(routes) > maxWireRoutes {
		return reject(RejectCompose, "wire.routes", "%d routes are declared; at most %d are allowed", len(routes), maxWireRoutes)
	}
	type address struct {
		service string
		port    int
	}
	seen := map[address]string{}
	for i, r := range routes {
		field := fmt.Sprintf("wire.routes[%d]", i)
		switch {
		case !m.has(r.Service):
			return reject(RejectCompose, field+".service", "service %q is not defined in compose.files", r.Service)
		case r.Service == d.Project.Builder:
			return reject(RejectCompose, field+".service", "service %q is the builder, which runs only in the build stage", r.Service)
		case slices.Contains(HarnessServices, r.Service):
			return reject(RejectCompose, field+".service", "service %q belongs to the hub harness", r.Service)
		case r.Port < 1 || r.Port > 65535:
			return reject(RejectCompose, field+".port", "port %d is outside 1..65535", r.Port)
		}
		at := address{r.Service, r.Port}
		if first, dup := seen[at]; dup {
			return reject(RejectCompose, field, "%s:%d is already routed at %s", r.Service, r.Port, first)
		}
		seen[at] = field
	}
	return nil
}

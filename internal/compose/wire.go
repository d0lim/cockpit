package compose

import (
	"fmt"
	"slices"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
)

const maxWireRoutes = 64

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

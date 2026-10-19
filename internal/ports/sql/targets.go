package sqlport

import (
	"fmt"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
)

const loopbackHost = "127.0.0.1"

func TargetsFor(decl contract.Declaration, base int) (map[string]Target, error) {
	if base <= 0 {
		return nil, fmt.Errorf("sql targets need the run's port base, got %d", base)
	}
	targets := make(map[string]Target, len(decl.Databases))
	for i, db := range decl.Databases {
		p, ok := compose.PortByName(decl.Ports, db.Port)
		if !ok {
			return nil, fmt.Errorf("databases[%d].port %q names no entry of ports", i, db.Port)
		}
		targets[string(db.Name)] = Target{Host: loopbackHost, Port: compose.HostPort(base, p), RootUser: db.RootUser, Database: db.Database}
	}
	return targets, nil
}

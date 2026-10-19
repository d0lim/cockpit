package compose

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
)

const (
	PortWidth      = 100
	CanaryOffset   = 99
	edgeListenBase = 30000
	maxPort        = 65535
)

var reservedOffsets = map[int]string{CanaryOffset: "the seal probe host canary"}

func checkOffsets(ports []contract.PortOffset) error {
	used := map[int]string{}
	for i, p := range ports {
		field := fmt.Sprintf("ports[%d]", i)
		if p.Offset < 0 || p.Offset >= PortWidth {
			return reject(RejectPorts, field+".offset", "offset %d is outside the port block of %d (0 to %d)", p.Offset, PortWidth, PortWidth-1)
		}
		if owner, ok := reservedOffsets[p.Offset]; ok {
			return reject(RejectPorts, field+".offset", "offset %d is reserved for %s", p.Offset, owner)
		}
		if first, ok := used[p.Offset]; ok {
			return reject(RejectPorts, field+".offset", "offset %d is already used by %s", p.Offset, first)
		}
		used[p.Offset] = field
		if p.Port < 1 || p.Port > maxPort {
			return reject(RejectPorts, field+".port", "port %d is not between 1 and %d", p.Port, maxPort)
		}
		if strings.TrimSpace(p.Service) == "" {
			return reject(RejectPorts, field+".service", "service is empty")
		}
	}
	return nil
}

func HostPort(base int, p contract.PortOffset) int {
	return base + p.Offset
}

func CanaryPort(base int) int {
	return base + CanaryOffset
}

func edgeListen(p contract.PortOffset) int {
	return edgeListenBase + p.Offset
}

func edgeRoutes(ports []contract.PortOffset) []string {
	out := make([]string, len(ports))
	for i, p := range ports {
		out[i] = strconv.Itoa(edgeListen(p)) + "=" + p.Service + ":" + strconv.Itoa(p.Port)
	}
	return out
}

func edgePublished(base int, ports []contract.PortOffset) []string {
	out := make([]string, len(ports))
	for i, p := range ports {
		out[i] = fmt.Sprintf("127.0.0.1:%d:%d", HostPort(base, p), edgeListen(p))
	}
	return out
}

func PortVar(p contract.PortOffset) string {
	return "RUN_PORT_" + strings.ToUpper(strings.ReplaceAll(string(p.Name), "-", "_"))
}

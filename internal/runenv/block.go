package runenv

import (
	"fmt"
	"net"
	"strconv"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/config"
)

type portFree func(port int) bool

func loopbackFree(port int) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

func FreeBlock(cfg config.Config, free portFree) (int, error) {
	for slot := 0; slot < cfg.Limits.Slots; slot++ {
		base := cfg.Limits.PortBase + slot*compose.PortWidth
		if blockUsable(cfg, base, free) {
			return base, nil
		}
	}
	return 0, fmt.Errorf("no free port block among %d slots from %d", cfg.Limits.Slots, cfg.Limits.PortBase)
}

func blockUsable(cfg config.Config, base int, free portFree) bool {
	for _, r := range cfg.ReservedPorts {
		if r.Overlaps(base, base+compose.PortWidth) {
			return false
		}
	}
	for port := base; port < base+compose.PortWidth; port++ {
		if !free(port) {
			return false
		}
	}
	return true
}

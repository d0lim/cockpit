package lock

import (
	"fmt"
	"net"
	"strconv"
)

const (
	ListenHost  = "127.0.0.1"
	DefaultPort = 7710
	EnvPort     = "COCKPIT_PORT"
)

func ListenAddr(getenv func(string) string) (string, int, error) {
	raw := getenv(EnvPort)
	if raw == "" {
		return net.JoinHostPort(ListenHost, strconv.Itoa(DefaultPort)), DefaultPort, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 || strconv.Itoa(port) != raw {
		return "", 0, fmt.Errorf("%s must be a port number between 1 and 65535; the hub listens on %s only", EnvPort, ListenHost)
	}
	return net.JoinHostPort(ListenHost, strconv.Itoa(port)), port, nil
}

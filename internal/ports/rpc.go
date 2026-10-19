package ports

import (
	"context"
	"encoding/json"
	"strconv"
)

const (
	PortSQL    = "sql"
	PortDocker = "docker"
	PortHTTP   = "http"
	PortSignal = "signal"
	PortState  = "state"
)

var HubPorts = []string{PortSQL, PortDocker, PortHTTP, PortSignal, PortState}

type Handler interface {
	Call(ctx context.Context, method string, args []json.RawMessage) (any, error)
}

func Arg(args []json.RawMessage, i int, target any, mask Mask, name string) error {
	if i >= len(args) || string(args[i]) == "null" {
		return BadArgs(mask, name+" is required (argument "+strconv.Itoa(i+1)+")")
	}
	if err := json.Unmarshal(args[i], target); err != nil {
		return BadArgs(mask, name+" does not match the expected shape")
	}
	return nil
}

func OptionalArg(args []json.RawMessage, i int, target any, mask Mask, name string) error {
	if i >= len(args) || string(args[i]) == "null" {
		return nil
	}
	return Arg(args, i, target, mask, name)
}

func UnknownMethod(mask Mask, port, method string) error {
	return BadArgs(mask, port+" has no method "+strconv.Quote(method))
}

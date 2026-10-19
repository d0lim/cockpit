package dockerport

import (
	"context"
	"encoding/json"

	"github.com/d0lim/cockpit/internal/ports"
)

func (p *Port) Call(ctx context.Context, method string, args []json.RawMessage) (any, error) {
	if method == "ps" {
		return p.PS(ctx)
	}
	var service string
	switch method {
	case "logs", "stats", "file", "exec":
		if err := ports.Arg(args, 0, &service, p.mask, "service"); err != nil {
			return nil, err
		}
	default:
		return nil, ports.UnknownMethod(p.mask, ports.PortDocker, method)
	}
	switch method {
	case "logs":
		req := LogsRequest{Service: service}
		if err := ports.OptionalArg(args, 1, &req, p.mask, "options"); err != nil {
			return nil, err
		}
		req.Service = service
		return p.Logs(ctx, req)
	case "stats":
		return p.Stats(ctx, service)
	case "file":
		req := FileRequest{Service: service}
		if err := ports.Arg(args, 1, &req.Path, p.mask, "path"); err != nil {
			return nil, err
		}
		var opts struct {
			MaxBytes int `json:"maxBytes"`
		}
		if err := ports.OptionalArg(args, 2, &opts, p.mask, "options"); err != nil {
			return nil, err
		}
		req.MaxBytes = opts.MaxBytes
		return p.File(ctx, req)
	}
	req := ExecRequest{Service: service}
	if err := ports.Arg(args, 1, &req.Argv, p.mask, "argv"); err != nil {
		return nil, err
	}
	var opts struct {
		TimeoutMs int64 `json:"timeoutMs"`
		MaxBytes  int   `json:"maxBytes"`
	}
	if err := ports.OptionalArg(args, 2, &opts, p.mask, "options"); err != nil {
		return nil, err
	}
	req.TimeoutMs, req.MaxBytes = opts.TimeoutMs, opts.MaxBytes
	return p.Exec(ctx, req)
}

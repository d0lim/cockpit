package sqlport

import (
	"context"
	"encoding/json"

	"github.com/d0lim/cockpit/internal/ports"
)

func (p *Port) Call(ctx context.Context, method string, args []json.RawMessage) (any, error) {
	if method != "query" {
		return nil, ports.UnknownMethod(p.mask, ports.PortSQL, method)
	}
	var req Request
	if err := ports.Arg(args, 0, &req.DB, p.mask, "db"); err != nil {
		return nil, err
	}
	if err := ports.Arg(args, 1, &req.Statement, p.mask, "statement"); err != nil {
		return nil, err
	}
	var opts struct {
		MaxRows   int               `json:"maxRows"`
		MaxBytes  int               `json:"maxBytes"`
		TimeoutMs int64             `json:"timeoutMs"`
		Params    []json.RawMessage `json:"params"`
	}
	if err := ports.OptionalArg(args, 2, &opts, p.mask, "options"); err != nil {
		return nil, err
	}
	req.MaxRows, req.MaxBytes, req.TimeoutMs, req.Params = opts.MaxRows, opts.MaxBytes, opts.TimeoutMs, decodeParams(opts.Params)
	return p.Query(ctx, req)
}

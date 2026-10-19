package dockerport

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/ports"
)

const statsBytes = 64 << 10

func (p *Port) Stats(ctx context.Context, service string) (map[string]string, error) {
	id, err := p.resolve(ctx, service)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	out := newCap(statsBytes, cancel)
	res, err := p.docker.Run(cctx, compose.Command{Args: []string{"stats", "--no-stream", "--no-trunc", "--format", "{{json .}}", id}, Stdout: out})
	raw, over := out.result()
	if over {
		return nil, ports.Limit(p.mask, "docker stats output exceeds the limit — "+service)
	}
	if err != nil || res.Code != 0 {
		return nil, p.failure(cctx, err, res, p.timeout, "docker stats "+service)
	}
	fields := map[string]any{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(raw))), &fields); err != nil {
		return nil, ports.Unavailable(p.mask, "docker stats output is not one JSON object — "+service)
	}
	stats := map[string]string{}
	for k, v := range fields {
		if s, ok := v.(string); ok {
			stats[k] = p.mask(s)
		}
	}
	return stats, nil
}

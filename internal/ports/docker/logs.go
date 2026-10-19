package dockerport

import (
	"context"
	"regexp"
	"strconv"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/ports"
)

var sinceValue = regexp.MustCompile(`^([0-9]+(\.[0-9]+)?|[0-9]+(ms|s|m|h)([0-9]+(ms|s|m|h))*|[0-9]{4}-[0-9]{2}-[0-9]{2}(T[0-9]{2}:[0-9]{2}(:[0-9]{2}(\.[0-9]{1,9})?)?(Z|[+-][0-9]{2}:[0-9]{2})?)?)$`)

type LogsRequest struct {
	Service  string `json:"service"`
	Since    string `json:"since,omitempty"`
	Tail     *int   `json:"tail,omitempty"`
	MaxBytes int    `json:"maxBytes,omitempty"`
}

func (p *Port) Logs(ctx context.Context, req LogsRequest) (TextOutput, error) {
	args := []string{"logs"}
	if req.Since != "" {
		if !sinceValue.MatchString(req.Since) {
			return TextOutput{}, ports.BadArgs(p.mask, "since must be a duration like 10m or an RFC 3339 time")
		}
		args = append(args, "--since="+req.Since)
	}
	if req.Tail != nil {
		if *req.Tail < 0 || *req.Tail > defaultMaxTail {
			return TextOutput{}, ports.BadArgs(p.mask, "tail must be between 0 and "+strconv.Itoa(defaultMaxTail))
		}
		args = append(args, "--tail="+strconv.Itoa(*req.Tail))
	}
	id, err := p.resolve(ctx, req.Service)
	if err != nil {
		return TextOutput{}, err
	}
	max := p.bytesFor(req.MaxBytes)
	cctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	stop, halt := context.WithCancel(cctx)
	defer halt()
	out := newCap(rawLimit(max), halt)
	res, err := p.docker.Run(stop, compose.Command{Args: append(args, id), Stdout: out, Stderr: out})
	raw, over := out.result()
	if over {
		return clip(raw, true, max, p.mask), nil
	}
	if err != nil || res.Code != 0 {
		return TextOutput{}, p.failure(cctx, err, compose.Result{Code: res.Code, Stderr: raw}, p.timeout, "docker logs "+req.Service)
	}
	return clip(raw, false, max, p.mask), nil
}

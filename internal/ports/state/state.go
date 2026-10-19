package stateport

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"unicode"

	"github.com/d0lim/cockpit/internal/ports"
)

const (
	MaxKey   = 256
	MaxValue = 64 << 10
)

type KV interface {
	KVGet(collector string) (map[string]any, error)
	KVPut(collector string, value map[string]any) error
}

type Port struct {
	Collector string
	Store     KV
	Mask      ports.Mask
	mu        sync.Mutex
}

func (p *Port) mask() ports.Mask {
	if p.Mask == nil {
		return ports.Identity
	}
	return p.Mask
}

func validKey(k string) bool {
	return k != "" && len(k) <= MaxKey && !strings.ContainsFunc(k, unicode.IsControl)
}

func (p *Port) Call(_ context.Context, method string, args []json.RawMessage) (any, error) {
	mask := p.mask()
	var key string
	if err := ports.Arg(args, 0, &key, mask, "key"); err != nil {
		return nil, err
	}
	if !validKey(key) {
		return nil, ports.BadArgs(mask, "a state key is 1 to 256 characters without control characters")
	}
	key = mask(key)
	p.mu.Lock()
	defer p.mu.Unlock()
	current, err := p.Store.KVGet(p.Collector)
	if err != nil {
		return nil, ports.Unavailable(mask, "cannot read the collector state")
	}
	switch method {
	case "get":
		if v, ok := current[key].(string); ok {
			return v, nil
		}
		return nil, nil
	case "set":
		var value string
		if err := ports.Arg(args, 1, &value, mask, "value"); err != nil {
			return nil, err
		}
		if len(value) > MaxValue {
			return nil, ports.Limit(mask, "the state value is larger than the limit")
		}
		current[key] = mask(value)
		if err := p.Store.KVPut(p.Collector, current); err != nil {
			return nil, ports.Limit(mask, "cannot store the collector state: "+err.Error())
		}
		return nil, nil
	}
	return nil, ports.UnknownMethod(mask, ports.PortState, method)
}

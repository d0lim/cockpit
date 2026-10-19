package sqlport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/d0lim/cockpit/internal/ports"
)

const (
	maxParams     = 100
	maxParamBytes = 64 << 10
	scalarSize    = 8
)

func decodeParams(raw []json.RawMessage) []any {
	if raw == nil {
		return nil
	}
	out := make([]any, len(raw))
	for i, r := range raw {
		out[i] = decodeParam(r)
	}
	return out
}

func decodeParam(r json.RawMessage) any {
	dec := json.NewDecoder(bytes.NewReader(r))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return r
	}
	n, ok := v.(json.Number)
	if !ok {
		return v
	}
	if i, err := n.Int64(); err == nil {
		return i
	}
	if f, err := n.Float64(); err == nil {
		return f
	}
	return r
}

func checkParams(params []any, mask ports.Mask) error {
	if len(params) > maxParams {
		return ports.Limit(mask, fmt.Sprintf("%d params exceed the limit of %d", len(params), maxParams))
	}
	size := 0
	for i, v := range params {
		switch x := v.(type) {
		case string:
			size += len(x)
		case nil, bool, int64, float64:
			size += scalarSize
		default:
			return ports.BadArgs(mask, fmt.Sprintf("params[%d] is %s; only strings, numbers, booleans and null are accepted", i, kindOf(v)))
		}
		if size > maxParamBytes {
			return ports.Limit(mask, fmt.Sprintf("params exceed the limit of %d bytes", maxParamBytes))
		}
	}
	return nil
}

func kindOf(v any) string {
	switch v.(type) {
	case map[string]any:
		return "an object"
	case []any:
		return "an array"
	}
	return "not a plain value"
}

func argumentMismatch(err error) bool {
	text := err.Error()
	return strings.Contains(text, "arguments, got") || strings.Contains(text, "bind message supplies")
}

package otlp

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
)

var hexIDKeys = map[string]int{
	"traceId": 16, "trace_id": 16,
	"spanId": 8, "span_id": 8,
	"parentSpanId": 8, "parent_span_id": 8,
}

func base64IDs(body []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		return nil, err
	}
	return json.Marshal(rewriteIDs(tree))
}

func rewriteIDs(v any) any {
	switch x := v.(type) {
	case []any:
		for i, e := range x {
			x[i] = rewriteIDs(e)
		}
	case map[string]any:
		for k, e := range x {
			if size, ok := hexIDKeys[k]; ok {
				if s, isString := e.(string); isString {
					x[k] = hexToBase64(s, size)
					continue
				}
			}
			x[k] = rewriteIDs(e)
		}
	}
	return v
}

func hexToBase64(s string, size int) string {
	if len(s) != 2*size {
		return s
	}
	raw, err := hex.DecodeString(s)
	if err != nil {
		return s
	}
	return base64.StdEncoding.EncodeToString(raw)
}

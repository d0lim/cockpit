package wire

import (
	"encoding/base64"
	"strings"
)

func decodeJWT(text string) (*JWT, bool) {
	parts := strings.Split(text, ".")
	if len(parts) != 3 {
		return nil, false
	}
	header, ok := jwtObject(parts[0])
	if !ok {
		return nil, false
	}
	claims, ok := jwtObject(parts[1])
	if !ok {
		return nil, false
	}
	if _, err := base64.RawURLEncoding.DecodeString(parts[2]); err != nil {
		return nil, false
	}
	return &JWT{Header: header, Claims: claims}, true
}

func jwtObject(segment string) (map[string]any, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return nil, false
	}
	v, err := decodeJSON(raw)
	if err != nil {
		return nil, false
	}
	object, ok := v.(map[string]any)
	return object, ok
}

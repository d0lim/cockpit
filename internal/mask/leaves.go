package mask

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
)

var secretKeyName = regexp.MustCompile(`(?i)^` + secretKey + `$`)

func (m *Masker) Leaves(v any) any {
	return m.leaf(v, false)
}

func (m *Masker) hiddenKey(k string) bool {
	return m.keys != nil && m.keys.MatchString(k[strings.LastIndexByte(k, '.')+1:])
}

func (m *Masker) leaf(v any, hidden bool) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		if hidden {
			return marker
		}
		return m.Mask(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = m.leaf(e, hidden)
		}
		return out
	case map[string]any:
		return maskKeys(m, x, func(k string, e any) any { return m.leaf(e, hidden || m.hiddenKey(k)) })
	case []string:
		out := make([]string, len(x))
		for i, e := range x {
			out[i] = m.leaf(e, hidden).(string)
		}
		return out
	case map[string]string:
		return maskKeys(m, x, func(k string, e string) string { return m.leaf(e, hidden || m.hiddenKey(k)).(string) })
	case map[string][]string:
		return maskKeys(m, x, func(k string, e []string) []string { return m.leaf(e, hidden || m.hiddenKey(k)).([]string) })
	}
	if scalar(v) {
		if hidden {
			return marker
		}
		return v
	}
	return m.leaf(roundTrip(v), hidden)
}

func scalar(v any) bool {
	if _, ok := v.(json.Number); ok {
		return true
	}
	switch reflect.ValueOf(v).Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func roundTrip(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return marker
	}
	var out any
	if json.Unmarshal(raw, &out) != nil {
		return marker
	}
	return out
}

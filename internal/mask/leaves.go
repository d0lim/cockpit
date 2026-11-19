package mask

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
)

var secretKeyName = regexp.MustCompile(`(?i)^` + secretKey + `$`)

func SecretKeyName(key string) bool { return secretKeyName.MatchString(key) }

type cover struct {
	hidden bool
	key    string
}

func (c cover) under(m *Masker, key string) cover {
	switch {
	case c.hidden:
		return cover{hidden: true}
	case m.hiddenKey(key):
		return cover{hidden: true, key: key}
	}
	return cover{}
}

func (m *Masker) Leaves(v any) any {
	return m.leaf(v, cover{})
}

func (m *Masker) hiddenKey(k string) bool {
	return m.keys != nil && m.keys.MatchString(k[strings.LastIndexByte(k, '.')+1:])
}

func (m *Masker) leaf(v any, c cover) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		if c.hidden && !m.publicValue(c.key, x) {
			return marker
		}
		return m.Mask(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = m.leaf(e, c)
		}
		return out
	case map[string]any:
		return maskKeys(m, x, func(k string, e any) any { return m.leaf(e, c.under(m, k)) })
	case []string:
		out := make([]string, len(x))
		for i, e := range x {
			out[i] = m.leaf(e, c).(string)
		}
		return out
	case map[string]string:
		return maskKeys(m, x, func(k string, e string) string { return m.leaf(e, c.under(m, k)).(string) })
	case map[string][]string:
		return maskKeys(m, x, func(k string, e []string) []string { return m.leaf(e, c.under(m, k)).([]string) })
	}
	if scalar(v) {
		if c.hidden {
			return marker
		}
		return v
	}
	return m.leaf(roundTrip(v), c)
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

package mask

import (
	"slices"
	"strconv"
)

func maskKeys[V any](m *Masker, in map[string]V, value func(key string, v V) V) map[string]V {
	out := make(map[string]V, len(in))
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var changed []string
	for _, k := range keys {
		if m.Mask(k) == k {
			out[k] = value(k, in[k])
		} else {
			changed = append(changed, k)
		}
	}
	for _, k := range changed {
		masked := m.Mask(k)
		name := masked
		for n := 2; ; n++ {
			if _, taken := out[name]; !taken {
				break
			}
			name = masked + "#" + strconv.Itoa(n)
		}
		out[name] = value(k, in[k])
	}
	return out
}

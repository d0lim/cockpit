package config

import (
	"fmt"
	"strings"
)

type PortRange struct {
	Name string
	From int
	To   int
}

func (r PortRange) Contains(port int) bool {
	return port >= r.From && port < r.To
}

func (r PortRange) Overlaps(from, to int) bool {
	return from < r.To && r.From < to
}

type portRangeFile struct {
	Name string `yaml:"name"`
	From *int   `yaml:"from"`
	To   *int   `yaml:"to"`
}

func reservedPorts(entries []portRangeFile) ([]PortRange, error) {
	out := make([]PortRange, 0, len(entries))
	names := map[string]int{}
	for i, e := range entries {
		field := fmt.Sprintf("reservedPorts[%d]", i)
		name := strings.TrimSpace(e.Name)
		if name == "" {
			return nil, fmt.Errorf("%s.name is empty", field)
		}
		if first, dup := names[name]; dup {
			return nil, fmt.Errorf("%s.name %q is already used by reservedPorts[%d]", field, name, first)
		}
		names[name] = i
		if e.From == nil || e.To == nil {
			return nil, fmt.Errorf("%s (%s) needs both from and to", field, name)
		}
		from, to := *e.From, *e.To
		for _, v := range []struct {
			key   string
			value int
		}{{"from", from}, {"to", to}} {
			if v.value < 1 || v.value > maxPort {
				return nil, fmt.Errorf("%s.%s (%s) must be between 1 and %d, got %d", field, v.key, name, maxPort, v.value)
			}
		}
		if from >= to {
			return nil, fmt.Errorf("%s (%s) must have from < to; the range is half-open [from, to), got [%d, %d)", field, name, from, to)
		}
		out = append(out, PortRange{Name: name, From: from, To: to})
	}
	return out, nil
}

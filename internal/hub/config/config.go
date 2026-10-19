package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

const (
	FileName     = "config.yaml"
	PortWidth    = 100
	maxFileBytes = 1 << 20
	maxPort      = 65535
	minPortBase  = 1024
)

type Limits struct {
	KeptPool   int
	RunningCap int
	QueueCap   int
	PortBase   int
	Slots      int
}

type Config struct {
	AllowedRepositories []string
	Limits              Limits
}

func Defaults() Limits {
	return Limits{KeptPool: 5, RunningCap: 3, QueueCap: 10, PortBase: 22000, Slots: 8}
}

func (l Limits) PortEnd() int {
	return l.PortBase + l.Slots*PortWidth - 1
}

type file struct {
	AllowedRepositories []string `yaml:"allowedRepositories"`
	Limits              struct {
		KeptPool   *int `yaml:"keptPool"`
		RunningCap *int `yaml:"runningCap"`
		QueueCap   *int `yaml:"queueCap"`
	} `yaml:"limits"`
	Ports struct {
		Base  *int `yaml:"base"`
		Slots *int `yaml:"slots"`
	} `yaml:"ports"`
}

func Load(home string) (Config, error) {
	path := filepath.Join(home, FileName)
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{Limits: Defaults()}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("cannot open hub config %s: %w", path, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return Config{}, fmt.Errorf("cannot read hub config %s: %w", path, err)
	}
	if len(data) > maxFileBytes {
		return Config{}, fmt.Errorf("hub config %s exceeds %d bytes", path, maxFileBytes)
	}
	cfg, err := Parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("hub config %s: %w", path, err)
	}
	return cfg, nil
}

func Parse(data []byte) (Config, error) {
	var raw file
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("cannot parse as YAML: %w", err)
	}
	limits := Defaults()
	settings := []struct {
		name  string
		value *int
		dst   *int
	}{
		{"limits.keptPool", raw.Limits.KeptPool, &limits.KeptPool},
		{"limits.runningCap", raw.Limits.RunningCap, &limits.RunningCap},
		{"limits.queueCap", raw.Limits.QueueCap, &limits.QueueCap},
		{"ports.base", raw.Ports.Base, &limits.PortBase},
		{"ports.slots", raw.Ports.Slots, &limits.Slots},
	}
	for _, s := range settings {
		if s.value == nil {
			continue
		}
		if *s.value <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive integer, got %d", s.name, *s.value)
		}
		*s.dst = *s.value
	}
	if limits.PortBase < minPortBase {
		return Config{}, fmt.Errorf("ports.base must be at least %d, got %d", minPortBase, limits.PortBase)
	}
	if limits.PortEnd() > maxPort {
		return Config{}, fmt.Errorf("ports.base %d with %d slots of %d ports ends at %d, past %d", limits.PortBase, limits.Slots, PortWidth, limits.PortEnd(), maxPort)
	}
	repos, err := repositories(raw.AllowedRepositories)
	if err != nil {
		return Config{}, err
	}
	return Config{AllowedRepositories: repos, Limits: limits}, nil
}

func repositories(entries []string) ([]string, error) {
	out := make([]string, 0, len(entries))
	for i, e := range entries {
		if !filepath.IsAbs(e) {
			return nil, fmt.Errorf("allowedRepositories[%d] must be an absolute path, got %q", i, e)
		}
		out = append(out, filepath.Clean(e))
	}
	return out, nil
}

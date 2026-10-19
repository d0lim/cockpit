package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/config"
)

func TestAbsentConfigFileYieldsTheCoreDefaultsAndNoAllowedRepositories(t *testing.T) {
	cfg, err := config.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := config.Limits{KeptPool: 5, RunningCap: 3, QueueCap: 10, PortBase: 22000, Slots: 8}
	if cfg.Limits != want || len(cfg.AllowedRepositories) != 0 {
		t.Fatalf("got %+v", cfg)
	}
	if cfg.Limits.PortEnd() != 22799 {
		t.Fatalf("default port block ends at %d", cfg.Limits.PortEnd())
	}
}

func TestValuesInConfigFileOverrideTheDefaultsAndUnsetValuesKeepThem(t *testing.T) {
	home := t.TempDir()
	data := "allowedRepositories:\n  - /src/toy\nlimits:\n  keptPool: 2\n  queueCap: 4\nports:\n  base: 30000\n  slots: 3\n"
	if err := os.WriteFile(filepath.Join(home, config.FileName), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	want := config.Limits{KeptPool: 2, RunningCap: 3, QueueCap: 4, PortBase: 30000, Slots: 3}
	if cfg.Limits != want || len(cfg.AllowedRepositories) != 1 || cfg.AllowedRepositories[0] != "/src/toy" {
		t.Fatalf("got %+v", cfg)
	}
}

func TestZeroOrNegativeLimitsAreRejectedWithTheirNames(t *testing.T) {
	fields := []struct{ yaml, name string }{
		{"limits:\n  keptPool: %s\n", "limits.keptPool"},
		{"limits:\n  runningCap: %s\n", "limits.runningCap"},
		{"limits:\n  queueCap: %s\n", "limits.queueCap"},
		{"ports:\n  base: %s\n", "ports.base"},
		{"ports:\n  slots: %s\n", "ports.slots"},
	}
	values := []string{"0", "-1"}
	evaluated := 0
	for _, f := range fields {
		for _, v := range values {
			_, err := config.Parse([]byte(strings.Replace(f.yaml, "%s", v, 1)))
			if err == nil || !strings.Contains(err.Error(), f.name) {
				t.Errorf("%s = %s: %v", f.name, v, err)
			}
			evaluated++
		}
		if _, err := config.Parse([]byte(strings.Replace(f.yaml, "%s", "2000", 1))); err != nil && !strings.Contains(f.name, "ports") {
			t.Errorf("%s = 2000 was rejected: %v", f.name, err)
		}
	}
	if evaluated != len(fields)*len(values) {
		t.Fatalf("evaluated %d cases", evaluated)
	}
}

func TestPortBlockThatEndsPastTheLastPortOrStartsBelow1024IsRejected(t *testing.T) {
	cases := map[string]string{
		"ports:\n  base: 65000\n  slots: 8\n": "ends at",
		"ports:\n  base: 80\n":                "at least 1024",
		"limits:\n  unknown: 1\n":             "unknown",
		"limits: [\n":                         "YAML",
	}
	for data, says := range cases {
		if _, err := config.Parse([]byte(data)); err == nil || !strings.Contains(err.Error(), says) {
			t.Errorf("%q: %v", data, err)
		}
	}
	if _, err := config.Parse([]byte("ports:\n  base: 65000\n  slots: 5\n")); err != nil {
		t.Fatalf("a block that ends at 65499 was rejected: %v", err)
	}
}

func TestHubConfigAndComposeAgreeOnThePortBlockWidth(t *testing.T) {
	if config.PortWidth != compose.PortWidth {
		t.Fatalf("config width %d, compose width %d", config.PortWidth, compose.PortWidth)
	}
}

func TestTheDefaultPortBlockStaysClearOfThePreviousHubsManualStackRange(t *testing.T) {
	const manualFirst, manualLast = 21000, 21799
	d := config.Defaults()
	if d.PortBase <= manualLast && d.PortEnd() >= manualFirst {
		t.Fatalf("default port block %d-%d intersects the manual stack range %d-%d", d.PortBase, d.PortEnd(), manualFirst, manualLast)
	}
}

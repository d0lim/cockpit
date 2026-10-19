package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

const (
	runA = "run-20261012-153000-997a37e"
	runB = "run-20261012-153000-997a37e-2"
)

func homeFixture(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "home")
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func fixture(id string, edit ...func(*Meta)) Meta {
	m := Meta{
		State:          lifecycle.State{Phase: lifecycle.Queued, Env: lifecycle.EnvAbsent},
		ID:             id,
		SHA:            "997a37e0c1d2b3a4f5e6d7c8b9a0f1e2d3c4b5a6",
		Suite:          "smoke",
		HostAPI:        1,
		CockpitVersion: "v0.3.0",
		Submitter:      "cli",
		CreatedAt:      "2026-10-12T06:30:00.000Z",
	}
	for _, e := range edit {
		e(&m)
	}
	return m
}

func slot(n int) *int { return &n }

func mustWriteMeta(t *testing.T, s *Store, m Meta) {
	t.Helper()
	if err := s.WriteMeta(m); err != nil {
		t.Fatal(err)
	}
}

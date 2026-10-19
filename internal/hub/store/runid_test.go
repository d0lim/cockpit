package store

import (
	"strings"
	"testing"
	"time"
)

const sha = "997a37e0c1d2b3a4f5e6d7c8b9a0f1e2d3c4b5a6"

func TestIsRunIDAcceptsTheRunIDShapeAndRejectsPathsCaseAndOverlongSequences(t *testing.T) {
	allow := []string{"run-20261012-153000-997a37e", "run-20261012-153000-997a37e-2", "run-20261012-153000-997a37e-12"}
	deny := []string{
		"../x",
		"run-20261012-153000-997a37e/../x",
		"RUN-20261012-153000-997a37e",
		"run-20261012-153000-997A37E",
		"run-20261012-153000-997a37e0",
		"run-20261012-1530000-997a37e",
		"run-20261012-153000-997a37e-" + strings.Repeat("1", 40),
		"run-20261012-153000-997a37e\n",
		"run-20261012-153000-997a37e-",
	}
	for _, id := range allow {
		if !IsRunID(id) {
			t.Errorf("%q rejected", id)
		}
	}
	for _, id := range deny {
		if IsRunID(id) {
			t.Errorf("%q accepted", id)
		}
	}
	if len(allow)+len(deny) != 12 {
		t.Fatal("case count")
	}
}

func TestNewRunIDUsesLocalTimeAndTheShortSHAAndNumbersCollisionsFromTwo(t *testing.T) {
	at := time.Date(2026, 10, 12, 15, 30, 0, 0, time.Local)
	base := "run-20261012-153000-997a37e"
	cases := []struct {
		existing []string
		want     string
	}{
		{nil, base},
		{[]string{base}, base + "-2"},
		{[]string{base, base + "-2"}, base + "-3"},
	}
	for _, c := range cases {
		if got := NewRunID(at, sha, c.existing); got != c.want || !IsRunID(got) {
			t.Fatalf("got %q, want %q", got, c.want)
		}
	}
	if got := NewRunID(time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local), sha, nil); got != "run-20260102-030405-997a37e" {
		t.Fatalf("zero padding: %q", got)
	}
}

package store

import (
	"path/filepath"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

func TestObserversSeeAppliedTransitionsOfTheirHomeOnlyAndStopWhenAsked(t *testing.T) {
	root := t.TempDir()
	mine, other := Open(filepath.Join(root, "a")), Open(filepath.Join(root, "b"))
	queued := Meta{State: lifecycle.State{Phase: lifecycle.Queued, Env: lifecycle.EnvAbsent}, ID: "run-20261019-010000-abc1234", HostAPI: 1, CreatedAt: "2026-10-19T01:00:00.000Z"}
	for _, s := range []*Store{mine, other} {
		if err := s.WriteMeta(queued); err != nil {
			t.Fatal(err)
		}
	}
	var seen []Transition
	stop := Observe(filepath.Join(root, "a"), func(tr Transition) { seen = append(seen, tr) })
	if _, err := mine.ApplyEvent(queued.ID, lifecycle.TickStart, lifecycle.Detail{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := other.ApplyEvent(queued.ID, lifecycle.TickStart, lifecycle.Detail{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := mine.ApplyEvent(queued.ID, lifecycle.Booted, lifecycle.Detail{}, nil); err == nil {
		t.Fatal("canary: booted from preparing was accepted")
	}
	if len(seen) != 1 || seen[0].Before.Phase != lifecycle.Queued || seen[0].After.Phase != lifecycle.Preparing || seen[0].Event != lifecycle.TickStart {
		t.Fatalf("seen %+v", seen)
	}
	stop()
	if _, err := mine.ApplyEvent(queued.ID, lifecycle.Abort, lifecycle.Detail{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 {
		t.Fatalf("an observer saw a transition after stop: %+v", seen)
	}
}

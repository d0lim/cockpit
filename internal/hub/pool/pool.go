package pool

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
)

const (
	DefaultPoolMax  = 5
	DefaultQueueMax = 10
	DefaultUpCap    = 3
)

type Limits struct {
	PoolMax  int
	QueueMax int
	UpCap    int
}

func DefaultLimits() Limits {
	return Limits{PoolMax: DefaultPoolMax, QueueMax: DefaultQueueMax, UpCap: DefaultUpCap}
}

func UpCapFrom(getenv func(string) string) int {
	v, err := strconv.Atoi(getenv("COCKPIT_UP_CAP"))
	if err != nil || v < 1 {
		return DefaultUpCap
	}
	return v
}

func StopOnDoneFrom(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv("COCKPIT_STOP_ON_DONE"))) {
	case "0", "false", "off":
		return false
	}
	return true
}

func IsPending(m store.Meta) bool {
	return m.Phase == lifecycle.Queued || m.Active()
}

func InPool(m store.Meta) bool {
	if m.Mark != lifecycle.MarkNone {
		return false
	}
	placed := m.Phase == lifecycle.Done || m.Phase == lifecycle.Stopping || m.Phase == lifecycle.Resuming
	return placed && (m.Env == lifecycle.EnvUp || m.Env == lifecycle.EnvStopped)
}

func IsUpPreserved(m store.Meta) bool {
	return m.Mark == lifecycle.MarkNone && ((m.Phase == lifecycle.Done && m.Env == lifecycle.EnvUp) || m.Phase == lifecycle.Resuming)
}

func Stoppable(m store.Meta) bool {
	return m.Mark == lifecycle.MarkNone && m.Phase == lifecycle.Done && m.Env == lifecycle.EnvUp
}

func KeepFrozen(m store.Meta) bool {
	return m.Mark != lifecycle.MarkNone || m.Phase == lifecycle.Destroying ||
		(m.Phase == lifecycle.Done && (m.Env == lifecycle.EnvAbsent || m.Env == lifecycle.EnvDestroyed))
}

type View struct {
	Runs      []store.Meta
	ForeignUp int
	OrphansUp int
	Cap       int
	PoolMax   int
}

func orElse(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func ByFinished(a, b store.Meta) int {
	return cmp.Or(cmp.Compare(orElse(a.FinishedAt, a.CreatedAt), orElse(b.FinishedAt, b.CreatedAt)), cmp.Compare(a.ID, b.ID))
}

func ByLastUp(a, b store.Meta) int {
	return cmp.Or(cmp.Compare(orElse(a.LastUpAt, a.FinishedAt), orElse(b.LastUpAt, b.FinishedAt)), cmp.Compare(a.ID, b.ID))
}

func filter(runs []store.Meta, keep func(store.Meta) bool) []store.Meta {
	var out []store.Meta
	for _, m := range runs {
		if keep(m) {
			out = append(out, m)
		}
	}
	return out
}

func Victim(runs []store.Meta, poolMax int) *store.Meta {
	kept := filter(runs, InPool)
	slices.SortStableFunc(kept, ByFinished)
	if len(kept) <= poolMax {
		return nil
	}
	for _, m := range kept[:len(kept)-1] {
		if !m.Keep && m.Phase == lifecycle.Done {
			return &m
		}
	}
	return nil
}

func UpCount(v View, activeShare bool) int {
	n := len(filter(v.Runs, IsUpPreserved)) + v.ForeignUp + v.OrphansUp
	if activeShare {
		n++
	}
	return n
}

func CapVictims(v View, activeShare bool) []store.Meta {
	over := UpCount(v, activeShare) - v.Cap
	if over <= 0 {
		return nil
	}
	up := filter(v.Runs, Stoppable)
	slices.SortStableFunc(up, ByLastUp)
	return up[:min(over, len(up))]
}

func StartBlock(v View, active bool) lifecycle.WaitReason {
	if active {
		return lifecycle.WaitActiveRun
	}
	if slices.ContainsFunc(v.Runs, func(m store.Meta) bool { return m.InProgress() }) {
		return lifecycle.WaitInProgress
	}
	kept := filter(v.Runs, InPool)
	if len(kept) > v.PoolMax {
		return lifecycle.WaitPoolOver
	}
	if len(kept) == v.PoolMax && !slices.ContainsFunc(kept, func(m store.Meta) bool { return !m.Keep }) {
		return lifecycle.WaitPoolAllKept
	}
	return lifecycle.WaitNone
}

type Candidate struct {
	ID         string `json:"id"`
	LastUpAt   string `json:"lastUpAt"`
	FinishedAt string `json:"finishedAt"`
	Keep       bool   `json:"keep"`
	Submitter  string `json:"submitter"`
}

func ResumeCandidates(runs []store.Meta) []Candidate {
	up := filter(runs, Stoppable)
	slices.SortStableFunc(up, ByLastUp)
	out := make([]Candidate, 0, len(up))
	for _, m := range up {
		out = append(out, Candidate{ID: m.ID, LastUpAt: m.LastUpAt, FinishedAt: m.FinishedAt, Keep: m.Keep, Submitter: m.Submitter})
	}
	return out
}

func CanResume(v View, activeShare bool) bool {
	return UpCount(v, activeShare)+1 <= v.Cap
}

package reconcile

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
)

type ReportOutcome struct {
	Verdict   lifecycle.Verdict
	Redaction *store.Redaction
}

type Env interface {
	KillGroup(pgid int) error
	RemoveOneOffs(ctx context.Context, id string) error
	CloseWithoutEnv(ctx context.Context, id string, down bool) error
	StopQuietly(ctx context.Context, id string)
	HasRunnerResult(id string) bool
	Report(ctx context.Context, m store.Meta) (ReportOutcome, error)
	Redo(ctx context.Context, id string) error
	Mask(id, text string) string
}

type Deps struct {
	Store          *store.Store
	Docker         Docker
	Env            Env
	Now            func() time.Time
	BootTime       func() time.Time
	Served         []int
	CockpitVersion string
}

type Hooks struct {
	Skip    map[string]bool
	RunOp   func(id string, fn func() error) error
	Managed *Managed
}

func (d Deps) now() string { return store.Timestamp(d.Now()) }

func Run(ctx context.Context, d Deps, h Hooks) (Scan, error) {
	listed, err := d.Store.ListRuns()
	if err != nil {
		return Scan{}, err
	}
	if err := d.Store.WriteState(store.NormalizedState(listed.Runs, listed.State, listed.StateBroken)); err != nil {
		return Scan{}, err
	}
	projects, err := d.Docker.ComposeLs(ctx)
	if err != nil {
		return Scan{}, err
	}
	statusOf := make(map[string]string, len(projects))
	for _, p := range projects {
		statusOf[p.Name] = p.Status
	}
	known := map[string]bool{}
	for _, m := range listed.Runs {
		known[m.ID] = true
		if h.Skip[m.ID] || m.Mark != lifecycle.MarkNone {
			continue
		}
		if err := reconcileRun(ctx, d, h, m, statusOf[m.ID]); err != nil {
			return Scan{}, err
		}
	}
	return ScanProjects(ctx, d.Docker, known, h.Managed, projects)
}

func reconcileRun(ctx context.Context, d Deps, h Hooks, m store.Meta, status string) error {
	switch {
	case m.Phase == lifecycle.Queued && !lifecycle.Serves(d.Served, m.HostAPI):
		return rejectUnserved(d, m)
	case m.Active():
		return closeInterrupted(ctx, d, m, status)
	case m.InProgress():
		return h.RunOp(m.ID, func() error { return redo(ctx, d, m) })
	case m.Phase == lifecycle.Done && (m.Env == lifecycle.EnvUp || m.Env == lifecycle.EnvStopped):
		return observePreserved(ctx, d, m, status)
	}
	return nil
}

type seen struct {
	observation lifecycle.Observation
	recreate    bool
}

func observe(ctx context.Context, d Deps, m store.Meta, status string) (seen, error) {
	found, err := d.Docker.Labelled(ctx, m.ID)
	if err != nil {
		return seen{}, err
	}
	switch {
	case found.Containers > 0 && Running(status):
		return seen{observation: lifecycle.ObservedUp}, nil
	case found.Containers > 0:
		return seen{observation: lifecycle.ObservedStopped}, nil
	case found.Volumes > 0:
		return seen{observation: lifecycle.ObservedStopped, recreate: true}, nil
	}
	return seen{observation: lifecycle.ObservedGone}, nil
}

func observePreserved(ctx context.Context, d Deps, m store.Meta, status string) error {
	s, err := observe(ctx, d, m, status)
	if err != nil {
		return err
	}
	next, err := d.Store.ApplyEvent(m.ID, lifecycle.Observed, lifecycle.Detail{Observation: s.observation}, func(x *store.Meta) {
		x.Recreate = s.recreate
		if s.observation == lifecycle.ObservedUp && m.Env != lifecycle.EnvUp {
			x.LastUpAt = d.now()
		}
	})
	if err != nil {
		return nil
	}
	if next.Mark == lifecycle.Lost {
		return d.Env.CloseWithoutEnv(ctx, m.ID, true)
	}
	return nil
}

func killStale(d Deps, m store.Meta, notes *[]string) {
	if m.Pgid == nil {
		return
	}
	startedAt := m.StartedAt
	if startedAt == "" {
		startedAt = m.CreatedAt
	}
	at, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil || at.Before(d.BootTime()) {
		*notes = append(*notes, fmt.Sprintf("pgid %d belongs to an earlier boot and was not killed", *m.Pgid))
		return
	}
	if err := d.Env.KillGroup(*m.Pgid); err != nil {
		*notes = append(*notes, fmt.Sprintf("could not kill pgid %d (%s)", *m.Pgid, d.Env.Mask(m.ID, err.Error())))
	}
}

func clearLeftovers(ctx context.Context, d Deps, m store.Meta, notes *[]string) {
	killStale(d, m, notes)
	if err := d.Env.RemoveOneOffs(ctx, m.ID); err != nil {
		*notes = append(*notes, fmt.Sprintf("could not remove one-off containers (%s)", d.Env.Mask(m.ID, err.Error())))
	}
}

func closeReporting(ctx context.Context, d Deps, m store.Meta) error {
	now := d.now()
	event := lifecycle.Fail
	detail := lifecycle.Detail{}
	var redaction *store.Redaction
	notice := ""
	if d.Env.HasRunnerResult(m.ID) && m.Slot != nil {
		out, err := d.Env.Report(ctx, m)
		if err == nil {
			event = lifecycle.Reported
			if out.Verdict != lifecycle.VerdictAborted {
				detail.Verdict = out.Verdict
			}
			redaction = out.Redaction
		} else {
			prefix := m.Notice
			if prefix == "" {
				prefix = "hub stopped"
			}
			notice = prefix + "; report failed: " + d.Env.Mask(m.ID, err.Error())
		}
	}
	next, err := d.Store.ApplyEvent(m.ID, event, detail, func(x *store.Meta) {
		x.FinishedAt = now
		if m.Env == lifecycle.EnvUp {
			x.LastUpAt = now
		}
		if redaction != nil {
			x.Redaction = redaction
		}
		if notice != "" {
			x.Notice = notice
		}
	})
	if err == nil && next.Phase == lifecycle.Done && next.Env == lifecycle.EnvAbsent {
		return d.Env.CloseWithoutEnv(ctx, m.ID, false)
	}
	return nil
}

func closeInterrupted(ctx context.Context, d Deps, m store.Meta, status string) error {
	notes := []string{fmt.Sprintf("hub stopped: the hub stopped during the %s stage", lifecycle.StageLabel(m.Phase))}
	clearLeftovers(ctx, d, m, &notes)
	s, err := observe(ctx, d, m, status)
	if err != nil {
		return err
	}
	next, err := d.Store.ApplyEvent(m.ID, lifecycle.Observed, lifecycle.Detail{Observation: s.observation}, func(x *store.Meta) {
		x.FailedStage = m.Phase
		x.Notice = strings.Join(notes, "; ")
		x.Pgid = nil
	})
	if err != nil {
		return nil
	}
	if next.Phase == lifecycle.Reporting {
		return closeReporting(ctx, d, next)
	}
	if next.Phase != lifecycle.Done {
		return nil
	}
	if _, err := d.Store.PatchMeta(m.ID, func(x *store.Meta) { x.FinishedAt = d.now() }); err != nil {
		return err
	}
	if next.Env == lifecycle.EnvAbsent {
		return d.Env.CloseWithoutEnv(ctx, m.ID, true)
	}
	if next.Env == lifecycle.EnvStopped && s.observation == lifecycle.ObservedUp {
		d.Env.StopQuietly(ctx, m.ID)
	}
	return nil
}

func rejectUnserved(d Deps, m store.Meta) error {
	notice := "submission pinned to a host API this hub does not serve, refused at hub start; submit again. " + lifecycle.HostAPINotice(m.HostAPI, d.Served, d.CockpitVersion)
	_, err := d.Store.ApplyEvent(m.ID, lifecycle.Reject, lifecycle.Detail{}, func(x *store.Meta) {
		x.Notice = notice
		x.WaitReason = lifecycle.WaitNone
		x.FinishedAt = d.now()
	})
	if err != nil {
		return nil
	}
	_, err = d.Store.UpdateState(func(st store.HubState) store.HubState { return st.Without(m.ID) })
	return err
}

func redo(ctx context.Context, d Deps, m store.Meta) error {
	var notes []string
	clearLeftovers(ctx, d, m, &notes)
	if len(notes) > 0 {
		if _, err := d.Store.PatchMeta(m.ID, func(x *store.Meta) { x.Notice = strings.Join(notes, "; ") }); err != nil {
			return err
		}
	}
	return d.Env.Redo(ctx, m.ID)
}

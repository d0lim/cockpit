package pipeline

import (
	"context"
	"fmt"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/hub/lease"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/hub/store"
)

type stageFunc func(ctx context.Context, l *live) (outcome, error)

func stageFor(p lifecycle.Phase) stageFunc {
	switch p {
	case lifecycle.Preparing:
		return prepare
	case lifecycle.Building:
		return build
	case lifecycle.Sealing:
		return sealStage
	case lifecycle.Booting:
		return boot
	case lifecycle.Testing:
		return runSuite
	case lifecycle.Reporting:
		return report
	}
	return nil
}

func (l *live) ensureLoaded(ctx context.Context) error {
	if l.loaded {
		return nil
	}
	m, err := l.st.MustRead(l.id)
	if err != nil {
		return err
	}
	loaded, err := l.h.declaration(ctx, m)
	if err != nil {
		return err
	}
	l.env.Loaded, l.env.Repo, l.loaded = loaded, compose.Git{Dir: m.Repository}, true
	l.st = store.Open(l.h.store.Home(), compose.SecretFilePatterns(loaded.Declaration)...)
	l.env.Docker = l.h.o.DockerFor(l.h.ops.PgidHooks(l.st, l.id))
	return nil
}

func prepare(ctx context.Context, l *live) (outcome, error) {
	if err := l.ensureLoaded(ctx); err != nil {
		return outcome{}, err
	}
	img, err := l.h.o.Ensure(ctx)
	if err != nil {
		return outcome{}, fmt.Errorf("cannot import the runner image: %w", err)
	}
	pin := contract.RunnerImage{Tag: img.Tag, ID: img.ID}
	if _, err := l.st.PatchMeta(l.id, func(m *store.Meta) { m.RunnerImage = &pin }); err != nil {
		return outcome{}, err
	}
	l.env.Image = img.Tag
	steps := []func() error{
		func() error { return l.env.Extract(ctx) },
		func() error { return l.env.GenerateEnv(ctx) },
		l.env.WriteRunFiles,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return outcome{}, err
		}
		if ctx.Err() != nil {
			return outcome{}, errAborted
		}
	}
	keys := l.env.Keys
	if _, err := l.st.PatchMeta(l.id, func(m *store.Meta) { m.ImageKeys = keys }); err != nil {
		return outcome{}, err
	}
	if err := l.env.SealStatic(ctx); err != nil {
		return outcome{}, err
	}
	l.say(fmt.Sprintf("prepared: %d static seal assertions passed", len(l.env.Static)))
	return outcome{event: lifecycle.Prepared}, nil
}

func build(ctx context.Context, l *live) (outcome, error) {
	if err := l.env.Build(ctx); err != nil {
		return outcome{}, err
	}
	return outcome{event: lifecycle.Built}, nil
}

func (l *live) edgePorts() map[string]int {
	ports := map[string]int{canaryName: compose.CanaryPort(l.env.Base)}
	for _, p := range l.env.Loaded.Declaration.Ports {
		ports[string(p.Name)] = compose.HostPort(l.env.Base, p)
	}
	return ports
}

func sealStage(ctx context.Context, l *live) (outcome, error) {
	if blocked := lease.BlockedPorts(l.edgePorts(), l.h.o.PortProbe); len(blocked) > 0 {
		return outcome{}, &stages.Failed{Message: fmt.Sprintf("ports are taken (%v); the block is not leased again", blocked)}
	}
	if _, err := l.env.SealProbes(ctx); err != nil {
		return outcome{}, err
	}
	l.say("sealed: the four probes failed on the sealed network and succeeded on egress")
	return outcome{event: lifecycle.Sealed}, nil
}

func boot(ctx context.Context, l *live) (outcome, error) {
	if err := l.env.Boot(ctx); err != nil {
		return outcome{}, err
	}
	return outcome{event: lifecycle.Booted, extra: func(m *store.Meta) { m.LastUpAt = store.Timestamp(l.h.o.Now()) }}, nil
}

func runSuite(ctx context.Context, l *live) (outcome, error) {
	m, err := l.st.MustRead(l.id)
	if err != nil {
		return outcome{}, err
	}
	pin, _, err := l.h.ops.EnsurePinned(ctx, l.env.Docker, m.RunnerImage)
	if err != nil {
		return outcome{}, err
	}
	if pin != *m.RunnerImage {
		if _, err := l.st.PatchMeta(l.id, func(x *store.Meta) { x.RunnerImage = &pin }); err != nil {
			return outcome{}, err
		}
	}
	res, err := l.env.RunSuite(ctx, m.Suite)
	l.runner = &res
	refreshErr := l.env.RefreshMasker()
	for _, line := range res.Lines {
		l.say(line)
	}
	switch {
	case err != nil:
		return outcome{}, err
	case refreshErr != nil:
		return outcome{}, refreshErr
	case res.Aborted:
		return outcome{}, errAborted
	case res.TimedOut:
		return outcome{}, &stages.Failed{Message: fmt.Sprintf("the suite passed its limit of %s", l.env.Limits.Suite)}
	}
	return outcome{event: lifecycle.Tested}, nil
}

func report(ctx context.Context, l *live) (outcome, error) {
	if err := l.ensureLoaded(ctx); err != nil {
		return outcome{}, err
	}
	m, err := l.st.MustRead(l.id)
	if err != nil {
		return outcome{}, err
	}
	out, notice, err := l.h.o.Reporter(context.WithoutCancel(ctx), envops.ReportInput{Meta: m, Root: l.env.Root, Runner: l.runner, Mask: l.env.Mask})
	if ctx.Err() != nil {
		if _, aerr := l.st.ApplyEvent(l.id, lifecycle.Abort, lifecycle.Detail{}, nil); aerr != nil {
			return outcome{}, aerr
		}
	}
	now := store.Timestamp(l.h.o.Now())
	if err != nil {
		return outcome{event: lifecycle.Fail, extra: func(x *store.Meta) {
			x.FailedStage, x.Notice, x.FinishedAt = lifecycle.Reporting, "report failed: "+l.env.Mask(err.Error()), now
		}}, nil
	}
	detail := lifecycle.Detail{}
	if out.Verdict != lifecycle.VerdictAborted {
		detail.Verdict = out.Verdict
	}
	return outcome{event: lifecycle.Reported, detail: detail, extra: func(x *store.Meta) {
		x.FinishedAt = now
		if x.Env == lifecycle.EnvUp {
			x.LastUpAt = now
		}
		if notice != "" {
			x.Notice = l.env.Mask(notice)
		}
		if out.Redaction != nil {
			x.Redaction = out.Redaction
		}
	}}, nil
}

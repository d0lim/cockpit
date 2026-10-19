package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
)

const (
	keepLines = 200
	tailLines = 50
)

type live struct {
	h      *Hub
	id     string
	st     *store.Store
	env    *stages.Env
	lines  []string
	stage  lifecycle.Phase
	runner *compose.RunnerResult
	boot   *contract.BootVerdict
	loaded bool
}

type outcome struct {
	event  lifecycle.Event
	detail lifecycle.Detail
	extra  func(*store.Meta)
}

var errAborted = errors.New("aborted")

func (l *live) remember(line string) {
	l.lines = append(l.lines, line)
	if len(l.lines) > keepLines {
		l.lines = l.lines[len(l.lines)-keepLines:]
	}
	l.h.o.Events(Event{Run: l.id, Phase: PhaseOut, Line: line})
}

func (l *live) say(line string) { l.remember(l.env.Mask(line)) }

func (h *Hub) begin(id string) (*live, error) {
	m, err := h.store.MustRead(id)
	if err != nil {
		return nil, err
	}
	if m.Phase != lifecycle.Preparing || m.Slot == nil {
		return nil, fmt.Errorf("run %s is not preparing with a leased block (%s)", id, m.Phase)
	}
	dir, err := h.store.RunDir(id)
	if err != nil {
		return nil, err
	}
	layout, err := compose.NewLayout(dir)
	if err != nil {
		return nil, err
	}
	root, err := compose.OpenRun(dir)
	if err != nil {
		return nil, err
	}
	l := &live{h: h, id: id, st: h.store}
	masker, err := mask.New(mask.Options{Mode: mask.Hub})
	if err != nil {
		root.Close()
		return nil, err
	}
	l.env = &stages.Env{
		RunID: id, Layout: layout, Root: root, Base: h.base(*m.Slot), RunnerUser: h.o.RunnerUser,
		Masker: masker, Limits: h.o.Limits, Now: h.o.Now, Log: l.remember,
	}
	l.env.Docker = h.o.DockerFor(h.ops.PgidHooks(h.store, id))
	return l, nil
}

func (h *Hub) Pipeline(ctx context.Context, id string) error {
	l, err := h.begin(id)
	if err != nil {
		return err
	}
	h.o.Events(Event{Run: id, Phase: PhaseStart})
	defer func() {
		os.RemoveAll(path.Join(l.env.Layout.Root, "build"))
		l.st.PatchMeta(id, func(m *store.Meta) { m.Pgid = nil })
		l.env.Root.Close()
		code := 1
		if m, err := l.st.MustRead(id); err == nil && m.Verdict == lifecycle.VerdictPassed {
			code = 0
		}
		h.o.Events(Event{Run: id, Phase: PhaseExit, Code: &code})
	}()
	for {
		m, err := l.st.MustRead(id)
		if err != nil {
			return err
		}
		stage := stageFor(m.Phase)
		if stage == nil {
			return nil
		}
		l.stage, l.lines = m.Phase, nil
		l.say("stage " + lifecycle.StageLabel(m.Phase))
		out, err := stage(ctx, l)
		if err == nil && m.Phase != lifecycle.Reporting && ctx.Err() != nil {
			err = errAborted
		}
		if err != nil {
			out = l.failure(ctx, err)
		}
		next, err := l.st.ApplyEvent(id, out.event, out.detail, out.extra)
		if err != nil {
			return nil
		}
		l.settle(ctx, m.Phase, next)
	}
}

func (l *live) patterns() []*regexp.Regexp {
	if !l.loaded {
		return nil
	}
	return compose.SecretFilePatterns(l.env.Loaded.Declaration)
}

func (l *live) tail() []string {
	var out []string
	for _, line := range l.lines {
		masked := l.env.Mask(line)
		if !store.CredentialPathOnly(masked, l.patterns()...) {
			out = append(out, masked)
		}
	}
	if len(out) > tailLines {
		out = out[len(out)-tailLines:]
	}
	return out
}

func (l *live) failure(ctx context.Context, err error) outcome {
	stage := l.stage
	if ctx.Err() != nil || errors.Is(err, errAborted) || (l.runner != nil && l.runner.Aborted) {
		return outcome{event: lifecycle.Abort, extra: func(m *store.Meta) {
			m.FailedStage = stage
			m.Notice = "aborted during the " + lifecycle.StageLabel(stage) + " stage"
		}}
	}
	var failed *stages.Failed
	detail := lifecycle.Detail{}
	if errors.As(err, &failed) && failed.Violation {
		detail.Verdict = lifecycle.VerdictFailed
	}
	tail, notice := l.tail(), l.env.Mask(err.Error())
	return outcome{event: lifecycle.Fail, detail: detail, extra: func(m *store.Meta) {
		m.FailedStage, m.Tail, m.Notice = stage, tail, notice
	}}
}

func (l *live) settle(ctx context.Context, from lifecycle.Phase, next store.Meta) {
	if next.Phase != lifecycle.Done {
		return
	}
	if next.FinishedAt == "" {
		l.st.PatchMeta(l.id, func(m *store.Meta) { m.FinishedAt = store.Timestamp(l.h.o.Now()) })
	}
	cleanup := context.WithoutCancel(ctx)
	switch {
	case next.Env == lifecycle.EnvAbsent:
		l.h.ops.CloseWithoutEnv(cleanup, l.id, from == lifecycle.Building || from == lifecycle.Sealing)
	case next.Env == lifecycle.EnvStopped && from == lifecycle.Booting:
		l.h.ops.StopQuietly(cleanup, l.id)
	}
}

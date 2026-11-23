package headless

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/pipeline"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/report"
)

const (
	tailLines  = 50
	shownLines = 3
)

var passEvents = map[lifecycle.Phase]lifecycle.Event{
	lifecycle.Preparing: lifecycle.Prepared,
	lifecycle.Building:  lifecycle.Built,
	lifecycle.Sealing:   lifecycle.Sealed,
	lifecycle.Booting:   lifecycle.Booted,
	lifecycle.Testing:   lifecycle.Tested,
}

func (r *run) walk(event lifecycle.Event, detail lifecycle.Detail) {
	var current *lifecycle.State
	if r.meta.Phase != "" {
		current = &r.meta.State
	}
	next, err := lifecycle.Transition(current, event, detail)
	if err != nil {
		return
	}
	if next.Phase != r.meta.Phase {
		at := store.Timestamp(r.deps.Now())
		r.lines = nil
		r.meta.EndStage(at, pipeline.StageOutcome(event))
		if next.Active() {
			r.meta.StartStage(next.Phase, at)
		}
	}
	r.meta.State = next
}

func (r *run) fail(ctx context.Context, stage lifecycle.Phase, err error) {
	event, detail := pipeline.FailureEvent(err, ctx.Err() != nil || (r.suiteRun != nil && r.suiteRun.Aborted))
	r.meta.FailedStage = stage
	if event == lifecycle.Abort {
		r.meta.Notice = "aborted during the " + lifecycle.StageLabel(stage) + " stage"
	} else {
		r.meta.Tail, r.meta.Notice = r.tail(), r.mask(err.Error())
	}
	r.walk(event, detail)
}

func (r *run) tail() []string {
	patterns := compose.SecretFilePatterns(r.loaded.Declaration)
	var out []string
	for _, line := range r.lines {
		if masked := r.mask(line); !store.CredentialPathOnly(masked, patterns...) {
			out = append(out, masked)
		}
	}
	return out
}

func (r *run) report(ctx context.Context) (string, error) {
	now := r.deps.Now()
	bundle, notice := pipeline.ReportBundle(pipeline.RunReport{
		Meta: r.meta, RunDir: r.env.Layout.Root, Ports: pipeline.RunPorts(r.loaded.Declaration, r.base),
		Runner: r.suiteRun, Boot: r.booted, Masker: r.env.Masker, Now: now,
	})
	bundle.Signals = r.stream
	out, err := pipeline.AssembleReport(context.WithoutCancel(ctx), bundle, pipeline.Recordings{Stream: r.stream, Exited: r.exited, Now: now})
	if ctx.Err() != nil {
		r.walk(lifecycle.Abort, lifecycle.Detail{})
	}
	if out.Redaction == nil && err != nil {
		r.meta.FailedStage, r.meta.Notice = lifecycle.Reporting, "report failed: "+r.mask(err.Error())
		r.walk(lifecycle.Fail, lifecycle.Detail{})
		return "", err
	}
	detail := lifecycle.Detail{}
	if v := lifecycle.Verdict(out.Judgement.Verdict); v != lifecycle.VerdictAborted {
		detail.Verdict = v
	}
	r.walk(lifecycle.Reported, detail)
	r.reasons = out.Judgement.Reasons
	if notice != "" {
		r.reasons = append([]string{notice}, r.reasons...)
	}
	if out.Redaction != nil {
		return "", errors.New(report.RefusedRedaction + ", so it was discarded: " + strings.Join(out.Redaction.Files, ", "))
	}
	r.bundle = out.Dir
	return fmt.Sprintf("verdict %s, bundle %s", out.Judgement.Verdict, out.Dir), nil
}

func (r *run) verdict() {
	if len(r.meta.Tail) > 0 {
		r.st.say("\nlast lines of the %s stage:", lifecycle.StageLabel(r.meta.FailedStage))
		for _, line := range r.meta.Tail {
			r.st.say("  %s", line)
		}
	}
	verdict := string(r.meta.Verdict)
	if verdict == "" {
		verdict = "none"
	}
	reasons := r.reasons
	if r.bundle == "" && r.meta.FailedStage != "" {
		verdict += fmt.Sprintf(", no report bundle: the run closed in the %s stage", lifecycle.StageLabel(r.meta.FailedStage))
		reasons = append([]string{r.meta.Notice}, reasons...)
	}
	r.st.say("\nverdict %s", verdict)
	shown := 0
	for _, reason := range reasons {
		if reason != "" && shown < shownLines {
			r.st.say("reason: %s", reason)
			shown++
		}
	}
}

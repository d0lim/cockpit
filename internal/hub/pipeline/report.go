package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/runfiles"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/seal"
	"github.com/d0lim/cockpit/internal/signal"
	"github.com/d0lim/cockpit/internal/version"
)

const (
	runnerLines = 200
	stageSource = "source"
)

func sealVerdict(runDir string) contract.SealVerdict {
	data, err := os.ReadFile(filepath.Join(runDir, seal.FileName))
	if err != nil {
		return contract.SealError
	}
	var doc struct {
		Verdict contract.SealVerdict `json:"verdict"`
	}
	if json.Unmarshal(data, &doc) != nil || doc.Verdict == "" {
		return contract.SealError
	}
	return doc.Verdict
}

func runnerOutcome(r *compose.RunnerResult, m *mask.Masker) *contract.RunnerOutcome {
	if r == nil {
		return nil
	}
	lines := r.Lines
	if len(lines) > runnerLines {
		lines = lines[len(lines)-runnerLines:]
	}
	out := &contract.RunnerOutcome{ContainerID: r.ContainerID, Code: r.Code, OOMKilled: r.OOMKilled, TimedOut: r.TimedOut, Aborted: r.Aborted, Lines: []string{}}
	for _, l := range lines {
		out.Lines = append(out.Lines, m.Mask(l))
	}
	return out
}

func suiteDir(in envops.ReportInput) string {
	return filepath.Join(in.RunDir, "work", "scripts")
}

func identityPreset(in envops.ReportInput, manifest *contract.Manifest) string {
	if manifest == nil {
		return ""
	}
	cockpitVersion := in.Meta.CockpitVersion
	if cockpitVersion == "" {
		cockpitVersion = version.String()
	}
	return IdentityMismatch(manifest, in.Meta.RunnerImage, cockpitVersion)
}

func DefaultReport(ctx context.Context, in envops.ReportInput) (reconcile.ReportOutcome, string, error) {
	if in.Masker == nil {
		return reconcile.ReportOutcome{}, "", errors.New("the run has no masker")
	}
	collected := report.ReadRunnerFiles(filepath.Join(in.RunDir, runfiles.E2EDir))
	files, stages := collected.Files, collected.Stages
	sources, sourceStage := report.SuiteSources(suiteDir(in), collected.Manifest.Value)
	for name, data := range sources {
		files[name] = data
	}
	stages[stageSource] = sourceStage
	if data, err := os.ReadFile(filepath.Join(in.RunDir, seal.FileName)); err == nil {
		files[seal.FileName] = data
	}
	meta := in.Meta
	notice := identityPreset(in, collected.Manifest.Value)
	if notice != "" && meta.Verdict == lifecycle.VerdictNone {
		meta.Verdict, meta.Notice = lifecycle.VerdictError, notice
	}
	inputs := report.Inputs{Manifest: collected.Manifest, Result: collected.Result, Boot: in.Boot, Seal: sealVerdict(in.RunDir)}
	if in.Runner != nil {
		inputs.RunnerCode, inputs.OOMKilled = in.Runner.Code, in.Runner.OOMKilled
	}
	pin := contract.RunnerImage{}
	if meta.RunnerImage != nil {
		pin = *meta.RunnerImage
	}
	summary := report.SummaryInput{
		RunID: meta.ID, Suite: meta.Suite, Commit: meta.SHA, Harness: compose.HarnessVersion, HubCommit: version.Commit(),
		HostAPI: meta.HostAPI, CockpitVersion: meta.CockpitVersion, RunnerImage: pin, Ports: in.Ports,
		ImageKeys: meta.ImageKeys, ImageDigests: meta.ImageDigests, Versions: map[string]string{"cockpit": version.String()},
		Stages: stages, Runner: runnerOutcome(in.Runner, in.Masker), GeneratedAt: store.Timestamp(in.Now),
	}
	bundle := report.Bundle{RunDir: in.RunDir, Meta: meta, Inputs: inputs, Summary: summary, Files: files, Mask: in.Masker,
		Patch: func(change func(*store.Meta)) error { _, err := in.Store.PatchMeta(meta.ID, change); return err }}
	stream, streamErr := signal.Open(in.RunDir, signal.Options{Masker: func() *mask.Masker { return in.Masker }})
	if streamErr == nil {
		defer stream.Close()
		bundle.Signals = stream
	}
	out, err := report.Assemble(bundle)
	if out.Redaction == nil && err != nil {
		return reconcile.ReportOutcome{}, "", err
	}
	if streamErr != nil {
		notice = joinNotice(notice, "the signal stream could not be opened: "+in.Masker.Mask(streamErr.Error()))
	}
	return reconcile.ReportOutcome{Verdict: lifecycle.Verdict(out.Judgement.Verdict), Redaction: out.Redaction}, joinNotice(notice, reasonsNotice(out.Judgement)), nil
}

func reasonsNotice(j report.Judgement) string {
	if j.Verdict == contract.VerdictPassed || len(j.Reasons) == 0 {
		return ""
	}
	return j.Reasons[0]
}

func joinNotice(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "" || a == b:
		return a
	}
	return a + "; " + b
}

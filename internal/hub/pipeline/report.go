package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

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
	"github.com/d0lim/cockpit/internal/version"
)

const (
	runnerLines = 200
	stageSource = "source"
	stageWire   = "wire record"
)

func sealOf(runDir string) (contract.SealVerdict, contract.NetworkLevel) {
	data, err := os.ReadFile(filepath.Join(runDir, seal.FileName))
	if err != nil {
		return contract.SealError, ""
	}
	var doc struct {
		Verdict contract.SealVerdict  `json:"verdict"`
		Network contract.NetworkLevel `json:"network"`
	}
	if json.Unmarshal(data, &doc) != nil || doc.Verdict == "" {
		return contract.SealError, ""
	}
	return doc.Verdict, doc.Network
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

func identityPreset(meta store.Meta, manifest *contract.Manifest) string {
	if manifest == nil {
		return ""
	}
	cockpitVersion := meta.CockpitVersion
	if cockpitVersion == "" {
		cockpitVersion = version.String()
	}
	return IdentityMismatch(manifest, meta.RunnerImage, cockpitVersion)
}

type RunReport struct {
	Meta   store.Meta
	RunDir string
	Ports  map[string]int
	Runner *compose.RunnerResult
	Boot   *contract.BootVerdict
	Masker *mask.Masker
	Now    time.Time
}

func RunPorts(decl contract.Declaration, base int) map[string]int {
	ports := map[string]int{}
	for _, p := range decl.Ports {
		ports[string(p.Name)] = compose.HostPort(base, p)
	}
	return ports
}

func ReportBundle(in RunReport) (report.Bundle, string) {
	collected := report.ReadRunnerFiles(filepath.Join(in.RunDir, runfiles.E2EDir))
	files, stages := collected.Files, collected.Stages
	sources, sourceStage := report.SuiteSources(filepath.Join(in.RunDir, "work", "scripts"), collected.Manifest.Value)
	for name, data := range sources {
		files[name] = data
	}
	stages[stageSource] = sourceStage
	if data, err := os.ReadFile(filepath.Join(in.RunDir, seal.FileName)); err == nil {
		files[seal.FileName] = data
	}
	meta := in.Meta
	notice := identityPreset(meta, collected.Manifest.Value)
	if notice != "" && meta.Verdict == lifecycle.VerdictNone {
		meta.Verdict, meta.Notice = lifecycle.VerdictError, notice
	}
	sealVerdict, network := sealOf(in.RunDir)
	inputs := report.Inputs{Manifest: collected.Manifest, Result: collected.Result, Boot: in.Boot, Seal: sealVerdict, Network: network}
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
	return report.Bundle{RunDir: in.RunDir, Meta: meta, Inputs: inputs, Summary: summary, Files: files, Mask: in.Masker}, notice
}

func DefaultReport(ctx context.Context, in envops.ReportInput) (reconcile.ReportOutcome, string, error) {
	if in.Masker == nil {
		return reconcile.ReportOutcome{}, "", errors.New("the run has no masker")
	}
	bundle, notice := ReportBundle(RunReport{Meta: in.Meta, RunDir: in.RunDir, Ports: in.Ports, Runner: in.Runner, Boot: in.Boot, Masker: in.Masker, Now: in.Now})
	bundle.Unread, bundle.Signals = collectorStages(in, bundle.Summary.Stages), in.Signals
	bundle.Patch = func(change func(*store.Meta)) error { _, err := in.Store.PatchMeta(bundle.Meta.ID, change); return err }
	out, err := AssembleReport(ctx, bundle, Recordings{Stream: in.Stream, Exited: in.RunnerExit, Now: in.Now, Wire: in.Wire})
	if out.Redaction == nil && err != nil {
		return reconcile.ReportOutcome{}, "", err
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

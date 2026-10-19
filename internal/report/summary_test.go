package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/contract"
)

func loadSchema(t *testing.T, name string) *jsonschema.Resolved {
	t.Helper()
	var s jsonschema.Schema
	if err := json.Unmarshal([]byte(fileText(t, filepath.Join(repoRoot(t), "schema", name+".json"))), &s); err != nil {
		t.Fatal(err)
	}
	r, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func assembled(t *testing.T, change func(*fixture)) (Outcome, map[string]any) {
	t.Helper()
	f := newFixture(t, masker(t, planted))
	if change != nil {
		change(f)
	}
	out, err := Assemble(f.bundle)
	if err != nil || out.Dir == "" {
		t.Fatalf("assemble: %v %+v", err, out)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(fileText(t, filepath.Join(out.Dir, FileSummary))), &doc); err != nil {
		t.Fatal(err)
	}
	return out, doc
}

func TestTheSummaryPassesTheFormat3SchemaAndStrictDecodeAndHasNoOpaKey(t *testing.T) {
	out, doc := assembled(t, nil)
	if err := loadSchema(t, "summary").Validate(doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["opa"]; ok || doc["format"] != float64(3) {
		t.Fatalf("format %v, opa present %v", doc["format"], ok)
	}
	var s contract.Summary
	if err := contract.Decode(bytes.NewReader([]byte(fileText(t, filepath.Join(out.Dir, FileSummary)))), &s); err != nil {
		t.Fatal(err)
	}
	if s.CockpitVersion != "0.1.0" || s.HostAPI != 1 || s.RunnerImage.Tag != "cockpit-runner:0.1.0" || len(s.Flows) != 3 {
		t.Fatalf("summary %+v", s)
	}
	doc["opa"] = map[string]any{"active": true}
	if err := loadSchema(t, "summary").Validate(doc); err == nil {
		t.Fatal("canary: the schema accepts an opa key")
	}
}

func TestTheBundleMetaPassesItsSchemaAndCarriesTheRunnerIdentity(t *testing.T) {
	out, _ := assembled(t, nil)
	var doc map[string]any
	if err := json.Unmarshal([]byte(fileText(t, filepath.Join(out.Dir, FileMeta))), &doc); err != nil {
		t.Fatal(err)
	}
	if err := loadSchema(t, "meta").Validate(doc); err != nil {
		t.Fatal(err)
	}
	image := doc["runnerImage"].(map[string]any)
	if doc["verdict"] != "failed" || doc["cockpitVersion"] != "0.1.0" || image["tag"] != "cockpit-runner:0.1.0" || doc["reportedAt"] != "2026-10-19T01:00:10.000Z" {
		t.Fatalf("meta %v", doc)
	}
}

func TestTheFlowTableCountsStepsAndStopsAtTheFirstFailure(t *testing.T) {
	in := with(func(in *Inputs) { in.Result.Value, in.RunnerCode = failedAtA2("boom"), intp(1) })
	flows := FlowSummaries(in.Manifest, in.Result)
	got := map[string][5]any{}
	for _, f := range flows {
		got[f.ID] = [5]any{f.Status, f.Passed, f.Failed, f.Skipped, f.StoppedAt}
	}
	want := map[string][5]any{
		"alpha": {contract.FlowOutcomeFailed, 1, 1, 1, "a2"},
		"beta":  {contract.FlowOutcomeSkipped, 0, 0, 2, ""},
		"gamma": {contract.FlowOutcomePassed, 1, 0, 0, ""},
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s: %v, want %v", id, got[id], w)
		}
	}
	c := StepCountsOf(in.Manifest, in.Result)
	if *c.Expected != 6 || *c.Actual != 6 || c.Failed != 1 || c.Skipped != 3 || c.Requests != 3 {
		t.Fatalf("counts %+v", c)
	}
	none := FlowSummaries(in.Manifest, ResultRead{Failure: ReadMissing})
	if len(none) != 3 || none[0].Status != contract.FlowOutcomeNotRun || none[0].StepList[0].Status != contract.StepStatusNotRun {
		t.Fatalf("flows without a result %+v", none[0])
	}
}

func TestAE6ACollectorFailureDoesNotChangeTheVerdictAndUnreadNamesItWithACount(t *testing.T) {
	failed := worker.Outcome{Status: worker.StatusKilled, Kind: "deadline", Failure: "deadline exceeded " + planted,
		Unread: []worker.UnreadRecord{{Source: "sql", Reason: "denied"}, {Source: "sql", Reason: "denied"}, {Source: "sql", Reason: "denied"}}}
	stage, unread := CollectorUnread("toy-db", failed)
	clean, _ := assembled(t, nil)
	out, doc := assembled(t, func(f *fixture) {
		f.bundle.Unread = unread
		f.bundle.Summary.Stages = map[string]contract.CollectStage{"toy-db": stage}
	})
	if out.Judgement.Verdict != clean.Judgement.Verdict || out.Judgement.Rule != clean.Judgement.Rule || !slices.Equal(out.Judgement.Reasons, clean.Judgement.Reasons) {
		t.Fatalf("verdict changed: %+v vs %+v", out.Judgement, clean.Judgement)
	}
	lines := []string{}
	for _, v := range doc["notProven"].(map[string]any)["unread"].([]any) {
		lines = append(lines, v.(string))
	}
	want := []string{"toy-db — collector killed: deadline", "toy-db · sql — denied (3 times)", "signals dropped (collector-cap): 4"}
	if !slices.Equal(lines, want) {
		t.Fatalf("unread %q", lines)
	}
	stages := doc["stages"].(map[string]any)
	if stages["toy-db"].(map[string]any)["status"] != "error" || stages[StageSignals].(map[string]any)["status"] != "ok" {
		t.Fatalf("stages %v", stages)
	}
	text := fileText(t, filepath.Join(out.Dir, FileNotProven))
	if !strings.Contains(text, HeadingUnread+"\n\n- toy-db — collector killed: deadline\n") || strings.Contains(text, planted) {
		t.Fatalf("not-proven.md %q", text)
	}
}

func TestAFailedFreezeIsAnUnreadRecordAndNotAnError(t *testing.T) {
	out, doc := assembled(t, func(f *fixture) { f.signals.err = os.ErrPermission })
	unread := doc["notProven"].(map[string]any)["unread"].([]any)
	if out.Judgement.Verdict != contract.VerdictFailed || len(unread) != 1 || unread[0] != "signals — freeze failed" {
		t.Fatalf("unread %v", unread)
	}
	if exists(filepath.Join(out.Dir, "signals")) {
		t.Fatal("a signals directory exists without a freeze")
	}
}

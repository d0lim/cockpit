package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

func bundleMeta(id string) contract.BundleMeta {
	slot := 3
	return contract.BundleMeta{
		ID: id, SHA: "abc1234", Suite: "toy", Harness: 8, HubCommit: "0e47e78", Submitter: "ci", CreatedAt: "2026-10-19T01:00:11.000Z",
		Verdict: contract.VerdictFailed, ReportedAt: "2026-10-19T01:09:00.000Z", CockpitVersion: "0.6.0", HostAPI: 1, Slot: &slot,
		ImageKeys: map[string]string{"IMG_APP": "abc"}, FailedStage: "testing", RunnerImage: contract.RunnerImage{Tag: "cockpit-runner:abc"},
	}
}

func staged(t *testing.T, s *Store) string {
	t.Helper()
	stage, err := s.ImportStage()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stage, "report"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "report", "summary.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return stage
}

func TestAnAdoptedRunIsListedAsDestroyedByImportWithoutSlotOrImagesAndTheHubStillStarts(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	s := Open(home)
	id := "run-20261019-010011-abc1234-h1"
	stage := staged(t, s)
	if err := s.Adopt(stage, ImportedMeta(bundleMeta(id))); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListRuns()
	if err != nil || len(list.Runs) != 1 {
		t.Fatalf("runs %+v, %v", list.Runs, err)
	}
	m := list.Runs[0]
	want := lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvDestroyed, Verdict: lifecycle.VerdictFailed}
	if m.State != want || m.DestroyReason != lifecycle.DestroyImported || m.Slot != nil || m.ImageKeys != nil || m.Repository != "" ||
		m.FinishedAt != "2026-10-19T01:09:00.000Z" || m.FailedStage != lifecycle.Testing || m.HarnessVersion != 8 {
		t.Fatalf("imported record %+v", m)
	}
	if _, err := os.Stat(filepath.Join(home, "runs", id, "report", "summary.json")); err != nil {
		t.Fatalf("the report did not move with the run: %v", err)
	}
	if err := RefuseLegacyLayout(home); err != nil {
		t.Fatalf("the imported record reads as the old layout: %v", err)
	}
	names, _ := os.ReadDir(filepath.Join(home, "runs"))
	if len(names) != 1 || strings.HasPrefix(names[0].Name(), importPrefix) {
		t.Fatalf("runs holds %v", names)
	}
}

func TestAdoptRefusesARunIDTheHubHoldsAndAStageOutsideItsRuns(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "home"))
	id := "run-20261019-010011-abc1234-h1"
	if err := s.Adopt(staged(t, s), ImportedMeta(bundleMeta(id))); err != nil {
		t.Fatalf("the first import is refused: %v", err)
	}
	second := staged(t, s)
	if err := s.Adopt(second, ImportedMeta(bundleMeta(id))); !errors.Is(err, ErrRunExists) {
		t.Fatalf("want ErrRunExists, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(second, "report", "summary.json")); err != nil {
		t.Fatalf("the refused stage was touched: %v", err)
	}
	outside := t.TempDir()
	if err := s.Adopt(outside, ImportedMeta(bundleMeta("run-20261019-010012-abc1234-h1"))); err == nil {
		t.Fatal("a directory outside the runs was adopted")
	}
	if m, _ := s.ReadMeta("run-20261019-010012-abc1234-h1"); m != nil {
		t.Fatalf("the outside directory became a run: %+v", m)
	}
}

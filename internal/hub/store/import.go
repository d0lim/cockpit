package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

const importPrefix = ".import-"

var ErrRunExists = errors.New("a run with this id is already in the hub")

var pipelineStages = []lifecycle.Phase{lifecycle.Preparing, lifecycle.Building, lifecycle.Sealing, lifecycle.Booting, lifecycle.Testing, lifecycle.Reporting}

func (s *Store) ImportStage() (string, error) {
	runs := filepath.Join(s.home, runsDir)
	for _, d := range []string{s.home, runs} {
		if err := ensurePrivateDir(d); err != nil {
			return "", err
		}
	}
	return os.MkdirTemp(runs, importPrefix)
}

func (s *Store) Adopt(stage string, m Meta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.RunDir(m.ID)
	if err != nil {
		return err
	}
	if filepath.Dir(stage) != filepath.Join(s.home, runsDir) || !strings.HasPrefix(filepath.Base(stage), importPrefix) {
		return fmt.Errorf("not an import stage of this hub: %s", filepath.Base(stage))
	}
	if _, err := os.Lstat(dir); err == nil {
		return ErrRunExists
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if leak := credentialPathAt(m, s.credentialFiles); leak != "" {
		return fmt.Errorf("meta does not record credential file paths: %s", leak)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := WriteAtomic(filepath.Join(stage, metaFile), append(data, '\n')); err != nil {
		return err
	}
	return os.Rename(stage, dir)
}

func ImportedMeta(b contract.BundleMeta) Meta {
	finished := b.FinishedAt
	if finished == "" {
		finished = b.ReportedAt
	}
	runner := b.RunnerImage
	m := Meta{
		State:          lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvDestroyed, Verdict: lifecycle.Verdict(b.Verdict)},
		ID:             b.ID,
		SHA:            b.SHA,
		Suite:          b.Suite,
		HostAPI:        b.HostAPI,
		HarnessVersion: b.Harness,
		CockpitVersion: b.CockpitVersion,
		RunnerImage:    &runner,
		Submitter:      b.Submitter,
		Wire:           b.Wire,
		CreatedAt:      b.CreatedAt,
		StartedAt:      b.StartedAt,
		FinishedAt:     finished,
		Tail:           b.Tail,
		Notice:         b.Notice,
		DestroyReason:  lifecycle.DestroyImported,
	}
	if stage := lifecycle.Phase(b.FailedStage); slices.Contains(pipelineStages, stage) {
		m.FailedStage = stage
	}
	return m
}

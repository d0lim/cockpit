package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/signal"
)

var frozenReadMutations = []porttest.Mutation{
	{"a destroyed run's frozen signals pass the run masker", "pass a masker with no known values to ReadFrozen", "red: the planted value is in the read", "TestADestroyedRunReadsItsSignalsFromTheFrozenCopyThroughTheRunMasker"},
	{"a destroyed run without a frozen copy is not found", "answer an empty bundle read when ReadFrozen finds no copy", "red: the read answers 200", "TestADestroyedRunReadsItsSignalsFromTheFrozenCopyThroughTheRunMasker"},
}

func TestFrozenReadMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "frozen_test.go", frozenReadMutations)
}

type destroyedSource struct{}

func (destroyedSource) Read(string, signal.ReadRequest) (signal.ReadResponse, error) {
	return signal.ReadResponse{}, signal.ErrRunDestroyed
}

func TestADestroyedRunReadsItsSignalsFromTheFrozenCopyThroughTheRunMasker(t *testing.T) {
	f := newFixture(t, func(d *Deps) {
		d.Signals = destroyedSource{}
		d.Mask = func(string) *mask.Masker { return hubMasker(t, planted) }
	})
	gone := lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvDestroyed, Verdict: lifecycle.VerdictPassed}
	f.seed(runMeta(1, gone, doneUp))
	f.seed(runMeta(2, gone, doneUp))
	copyDir := filepath.Join(f.runDir(rid(1)), report.DirReport, signal.Dir)
	os.MkdirAll(copyDir, 0o700)
	line := `{"seq":1,"ts":"2026-10-19T01:00:00.000Z","tsSource":"host","kind":"event","collector":"toy-db","id":"orders:7","attrs":{"table":"orders","ref.order":"ord-7"},"event":{"body":"note ` + planted + `"}}` + "\n"
	os.WriteFile(filepath.Join(copyDir, "stream.jsonl"), []byte(line), 0o600)
	status, raw := f.get("/api/runs/" + rid(1) + "/signals")
	text := string(raw)
	if status != http.StatusOK || strings.Contains(text, planted) || !strings.Contains(text, "note ****") || !strings.Contains(text, `"source":"bundle"`) || !strings.Contains(text, "ord-7") {
		t.Fatalf("frozen read: %d %s", status, raw)
	}
	if status, raw := f.get("/api/runs/" + rid(2) + "/signals"); status != http.StatusNotFound {
		t.Fatalf("a destroyed run without a copy: %d %s", status, raw)
	}
}

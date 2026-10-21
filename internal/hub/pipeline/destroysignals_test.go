package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/signal"
)

var destroySignalsMutations = []porttest.Mutation{
	{"destroy removes the live signal stream", "drop the Streams.Remove call in envops.Destroy", "red: runs/<id>/signals is still there after destroy", "TestDestroyRemovesTheLiveSignalStreamAndLaterReadsComeFromTheFrozenCopy"},
	{"a destroyed run's stream is never opened again", "drop the destroyed check in Registry.Get", "red: the API read after destroy answers from a recreated live stream in runs/<id>/signals", "TestDestroyRemovesTheLiveSignalStreamAndLaterReadsComeFromTheFrozenCopy"},
}

func TestDestroySignalsMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "destroysignals_test.go", destroySignalsMutations)
}

func TestDestroyRemovesTheLiveSignalStreamAndLaterReadsComeFromTheFrozenCopy(t *testing.T) {
	w, url := spanWorld(t, false)
	m := w.submit()
	if m.Phase != lifecycle.Done || m.Env != lifecycle.EnvUp {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	dir, _ := w.hub.Store().RunDir(m.ID)
	live := filepath.Join(dir, signal.Dir)
	frozen := filepath.Join(dir, report.DirReport, signal.Dir, "stream.jsonl")
	before := len(readSignals(t, url, m.ID).Signals)
	if before == 0 {
		t.Fatal("the live read holds no signal before destroy")
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("no live stream before destroy: %v", err)
	}
	if r := w.sched.Destroy(m.ID, scheduler.Actor{By: "tester"}); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	if got := w.meta(m.ID); got.Env != lifecycle.EnvDestroyed {
		t.Fatalf("state after destroy %+v", got.State)
	}
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Fatalf("runs/<id>/signals after destroy: %v", err)
	}
	if _, err := os.Stat(frozen); err != nil {
		t.Fatalf("the frozen copy went with the destroy: %v", err)
	}
	after := readSignals(t, url, m.ID)
	if after.Source != api.SourceBundle || !after.Frozen || len(after.Signals) == 0 || len(after.Signals) > before {
		t.Fatalf("signal read after destroy: source %q frozen %v %d signals", after.Source, after.Frozen, len(after.Signals))
	}
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Fatalf("the read after destroy recreated runs/<id>/signals: %v", err)
	}
	if _, err := w.hub.Streams().Get(m.ID); err != signal.ErrRunDestroyed {
		t.Fatalf("registry get after destroy: %v", err)
	}
}

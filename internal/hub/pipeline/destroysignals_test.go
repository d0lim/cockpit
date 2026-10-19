package pipeline

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/signal"
)

var destroySignalsMutations = []porttest.Mutation{
	{"destroy removes the live signal stream", "drop the Streams.Remove call in envops.Destroy", "red: runs/<id>/signals is still there after destroy", "TestDestroyRemovesTheLiveSignalStreamKeepsTheFrozenCopyAndLaterReadsAreNotFound"},
	{"a destroyed run's stream is never opened again", "drop the destroyed check in Registry.Get", "red: the API read after destroy answers 200 and recreates runs/<id>/signals", "TestDestroyRemovesTheLiveSignalStreamKeepsTheFrozenCopyAndLaterReadsAreNotFound"},
}

func TestDestroySignalsMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "destroysignals_test.go", destroySignalsMutations)
}

func TestDestroyRemovesTheLiveSignalStreamKeepsTheFrozenCopyAndLaterReadsAreNotFound(t *testing.T) {
	w, url := spanWorld(t, false)
	m := w.submit()
	if m.Phase != lifecycle.Done || m.Env != lifecycle.EnvUp {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	dir, _ := w.hub.Store().RunDir(m.ID)
	live := filepath.Join(dir, signal.Dir)
	frozen := filepath.Join(dir, report.DirReport, signal.Dir, "stream.jsonl")
	if n := len(readSignals(t, url, m.ID).Signals); n == 0 {
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
	res, err := http.Get(url + "/api/runs/" + m.ID + "/signals")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("signal read after destroy answered %d", res.StatusCode)
	}
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Fatalf("the read after destroy recreated runs/<id>/signals: %v", err)
	}
	if _, err := w.hub.Streams().Get(m.ID); err != signal.ErrRunDestroyed {
		t.Fatalf("registry get after destroy: %v", err)
	}
}

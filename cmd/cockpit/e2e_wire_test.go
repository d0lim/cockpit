package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/runnerbin"
	"github.com/d0lim/cockpit/internal/wire"
)

var wireRunMutations = []porttest.Mutation{
	{"every call reaches the record", "drop the fifth call in store.add", "red: the relay step has one call fewer and the record counts differ", "TestIntegrationAWireRunRecordsEveryCallWithoutACredentialAndARunWithoutWireStaysDirect"},
	{"the record masker holds the run's values", "build recordMasker without the value sources and the captured values", "red: the database password and the captured reference are in the record, the API and the screen", "TestIntegrationAWireRunRecordsEveryCallWithoutACredentialAndARunWithoutWireStaysDirect"},
	{"an open call stays in the take as incomplete", "leave the open calls out in store.seal", "red: the held call is missing from the aborted run", "TestIntegrationAWireRunRecordsEveryCallWithoutACredentialAndARunWithoutWireStaysDirect"},
	{"a value the suite captured stays masked in later takes", "start loadWireValues from an empty list", "red: the reference is in the take of the exec", "TestIntegrationAWireRunRecordsEveryCallWithoutACredentialAndARunWithoutWireStaysDirect"},
}

func TestWireRunMutationRecordMatchesTheTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "e2e_wire_test.go", wireRunMutations)
}

func needSealedDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("COCKPIT_INTEGRATION") != "1" {
		t.Skip("needs COCKPIT_INTEGRATION=1, a Docker daemon and scripts/build-runners.sh")
	}
	if _, err := ensureImage(t.Context(), runnerbin.FilePins{Path: filepath.Join(t.TempDir(), pinsFile)}); errors.Is(err, runnerbin.ErrNotEmbedded) {
		t.Skip("the runner is not embedded; run scripts/build-runners.sh first")
	}
}

func TestIntegrationAWireRunRecordsEveryCallWithoutACredentialAndARunWithoutWireStaysDirect(t *testing.T) {
	needSealedDocker(t)
	e := newE2E(t, "toy-wire")
	e.startHub()

	run := e.submit("--wire", "--suite", "wire")
	stopWatching := run.watchReads()
	verdict := run.wait()
	reads := stopWatching()
	t.Logf("%d reads of the record while the run went from queued to done", reads)
	if verdict != "passed" {
		t.Fatalf("verdict %s", verdict)
	}
	run.assertSealPassed()
	run.assertSuiteTake()
	values := run.credentialValues()
	surfaces := run.surfaces(wire.SuiteTake)
	surfaces.assertClean(t, "after the report", values)
	surfaces.assertVisible(t, "tok-0a1b2c3d4e5f", run.publicTicketID())
	run.assertScreenShowsWire()

	takes := []wantedTake{{wire.SuiteTake, wireSuiteCalls, len(wireOutsideCalls)}}
	run.ok("keep")
	takes = append(takes, wantedTake{run.execFlow("relay"), wireRelayCalls, 0})
	recallTake := run.execFlow("recall")
	takes = append(takes, wantedTake{recallTake, wireRecallCalls, 0})
	if got := field(t, run.pickedIn(recallTake, "recall", "latest", 0).Response.Body, "latest"); got != hidden {
		t.Error("the value the suite captured is not masked in the take of a later exec")
	}
	run.assertTakesStillRead("after the execs", values, takes)

	run.ok("stop")
	run.assertTakesStillRead("after stop", values, takes)
	run.ok("resume")
	takes = append(takes, wantedTake{run.execFlow("relay"), wireRelayCalls, 0})
	run.assertTakesStillRead("after resume and another exec", values, takes)
	run.ok("destroy", "--force")
	run.assertTakesStillRead("after destroy", values, takes)

	direct := e.submit("--suite", "wire", "--new")
	if verdict := direct.wait(); verdict != "passed" {
		t.Fatalf("the run without wire ended %s", verdict)
	}
	direct.assertWithoutWire()
	direct.ok("destroy", "--force")

	held := e.submit("--wire", "--suite", "wirehold", "--new")
	stopWatching = held.watchReads()
	held.awaitLog("ledger", "/hold")
	started := time.Now()
	held.ok("abort")
	if verdict := held.wait(); verdict != "aborted" {
		t.Fatalf("the held run ended %s", verdict)
	}
	stopWatching()
	t.Logf("abort to verdict: %s", time.Since(started).Round(time.Millisecond))
	held.assertHeldCallIsIncomplete()
	held.ok("destroy", "--force")
}

type wantedTake struct {
	name    string
	calls   []wireStepCalls
	outside int
}

func (w wireRun) assertTakesStillRead(when string, values []string, takes []wantedTake) {
	t := w.e.t
	t.Helper()
	names := make([]string, 0, len(takes))
	for _, take := range takes {
		names = append(names, take.name)
		w.assertTake(take.name, take.calls, take.outside)
		if status, _ := w.get("/runs/" + w.id + "/wire?take=" + take.name); status != http.StatusOK {
			t.Errorf("%s: the screen of take %s answered %d", when, take.name, status)
		}
	}
	listed := slices.Clone(names)
	slices.Sort(listed)
	if got := w.view("").Takes; !slices.Equal(got, listed) {
		t.Fatalf("%s: takes %v, want %v", when, got, listed)
	}
	_, page := w.get("/runs/" + w.id + "/wire")
	for _, name := range names {
		if !strings.Contains(page, ">"+name+"<") {
			t.Errorf("%s: the take picker lacks %s", when, name)
		}
	}
	surfaces := w.surfaces(names...)
	surfaces.assertClean(t, when, values)
	surfaces.assertVisible(t, "tok-0a1b2c3d4e5f", w.publicTicketID())
}

func (w wireRun) assertWithoutWire() {
	t := w.e.t
	t.Helper()
	if id := w.e.container(w.id, "cockpit-wire"); id != "" {
		t.Errorf("a run without wire has the recorder container %s", id)
	}
	if url := w.e.containerEnv(w.id, "app", "LEDGER_URL"); url != "http://ledger:8081" {
		t.Errorf("the app reaches the ledger at %q", url)
	}
	if !strings.Contains(w.e.logs(w.id, "ledger"), "/entries") {
		t.Error("the ledger log does not show the call the app made directly")
	}
	if status, _ := w.get("/api/runs/" + w.id + "/wire"); status != http.StatusNotFound {
		t.Errorf("the API answered %d", status)
	}
	if status, _ := w.get("/runs/" + w.id + "/wire"); status != http.StatusNotFound {
		t.Errorf("the screen answered %d", status)
	}
	_, page := w.get("/runs/" + w.id)
	if strings.Contains(page, "/runs/"+w.id+"/wire") {
		t.Error("the overview of a run without wire links the Wire screen")
	}
	if !strings.Contains(page, w.id) {
		t.Error("the overview was not rendered")
	}
	if w.summary().Wire != nil {
		t.Error("the summary of a run without wire carries a wire status")
	}
}

func (w wireRun) assertHeldCallIsIncomplete() {
	t := w.e.t
	t.Helper()
	if s := w.summary().Wire; s == nil || s.State != wire.StateTaken {
		t.Fatalf("summary wire %+v", s)
	}
	view := w.view("?scope=")
	heads := map[string]api.WireHead{}
	for _, step := range view.Steps {
		for _, h := range w.view("?flow=" + step.Flow + "&step=" + step.Step).Calls {
			heads[h.Path] = h
		}
	}
	if held, ok := heads["/hold"]; !ok || held.Outcome != wire.OutcomeIncomplete {
		t.Fatalf("the held call reads %+v in %+v", held, heads)
	}
	call := w.call(wire.SuiteTake, heads["/hold"].Seq)
	if !call.Call.Finished.IsZero() {
		t.Errorf("an incomplete call has the finish time %v", call.Call.Finished)
	}
	if _, err := os.Stat(filepath.Join(w.dir(), "report", "summary.json")); err != nil {
		t.Error(err)
	}
}

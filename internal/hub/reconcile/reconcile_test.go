package reconcile

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
)

func TestKeptUpRunThatComposeListsAsExitedIsMarkedStoppedAndNotResumed(t *testing.T) {
	w := setup(t, preserved(1, 10))
	w.docker.ls = []Project{{Name: rid(1), Status: "exited(12)"}}
	w.run(t)
	if m := w.read(t, rid(1)); m.Phase != lifecycle.Done || m.Env != lifecycle.EnvStopped {
		t.Fatalf("%+v", m.State)
	}
	if got := own(w.env.calls, rid(1)); len(got) != 0 {
		t.Fatalf("env calls %v", got)
	}
}

func TestKeptRunWithNoContainersNetworksOrVolumesIsLostAndClosedWithDown(t *testing.T) {
	w := setup(t, preserved(1, 10))
	w.docker.ls = []Project{}
	w.docker.labelled[rid(1)] = Labelled{}
	w.run(t)
	if m := w.read(t, rid(1)); m.Mark != lifecycle.Lost || m.Env != lifecycle.EnvAbsent {
		t.Fatalf("%+v", m.State)
	}
	if got := own(w.env.calls, rid(1)); !slices.Equal(got, []string{rid(1) + " closeWithoutEnv down=true"}) {
		t.Fatalf("%v", got)
	}
}

func TestKeptRunWithOnlyVolumesLeftIsStoppedForRecreationAndNotDestroyed(t *testing.T) {
	w := setup(t, preserved(1, 10, func(m *store.Meta) { m.Keep = true }))
	w.docker.ls = []Project{}
	w.docker.labelled[rid(1)] = Labelled{Volumes: 3}
	w.run(t)
	m := w.read(t, rid(1))
	if m.Phase != lifecycle.Done || m.Env != lifecycle.EnvStopped || !m.Recreate || m.Mark != lifecycle.MarkNone {
		t.Fatalf("%+v", m)
	}
	if got := own(w.env.calls, rid(1)); len(got) != 0 {
		t.Fatalf("env calls %v", got)
	}
}

func TestUnlabelledProjectsAreLeftAloneAndLabelledProjectsWithoutRecordsAreOnlyMarkedOrphans(t *testing.T) {
	w := setup(t)
	handMade := "run-env-20261012-164840-f837814"
	w.docker.ls = []Project{{Name: "acme", Status: "running(14)"}, {Name: handMade, Status: "running(12)"}}
	w.docker.labelled["acme"] = Labelled{Volumes: 4}
	w.docker.labelled[handMade] = Labelled{Containers: 12, Networks: 2, Volumes: 3}
	scan := w.run(t)
	if !reflect.DeepEqual(scan.Orphans, []Orphan{{Project: handMade, Running: true}}) {
		t.Fatalf("%+v", scan.Orphans)
	}
	if len(own(w.env.calls, "acme")) != 0 || len(own(w.env.calls, handMade)) != 0 {
		t.Fatalf("env calls %v", w.env.calls)
	}
}

func TestRunningUnrelatedComposeProjectAddsOneToTheRunningCountWhateverItsPorts(t *testing.T) {
	w := setup(t, preserved(1, 10))
	w.docker.ls = []Project{
		{Name: rid(1), Status: "running(12)"},
		{Name: "toy-shop", Status: "running(3)"},
		{Name: "acme-db", Status: "exited(1)"},
		{Name: "app-old-cockpit-run", Status: "created(1), running(2)"},
	}
	scan := w.run(t)
	if scan.ForeignUp != 2 || len(scan.Orphans) != 0 {
		t.Fatalf("%+v", scan)
	}
	w.docker.ls = w.docker.ls[:1]
	if again := w.run(t); again.ForeignUp != 0 {
		t.Fatalf("canary: %+v", again)
	}
}

func TestRunDiedWhileDestroyingKillsItsGroupAndOneOffsBeforeRedoingTheDestroy(t *testing.T) {
	w := setup(t, preserved(1, 10, func(m *store.Meta) {
		m.Phase = lifecycle.Destroying
		m.DestroyReason = lifecycle.DestroyPerson
		m.Pgid = new(777)
	}))
	w.run(t)
	if !slices.Equal(w.env.killed, []int{777}) {
		t.Fatalf("killed %v", w.env.killed)
	}
	if !slices.Equal(w.env.calls, []string{"kill 777", rid(1) + " removeOneOffs", rid(1) + " redo"}) {
		t.Fatalf("%v", w.env.calls)
	}
}

func TestRunDiedWhileStoppingRedoesTheStop(t *testing.T) {
	w := setup(t, preserved(1, 10, func(m *store.Meta) { m.Phase = lifecycle.Stopping; m.StopReason = lifecycle.StopCap }))
	w.run(t)
	if got := own(w.env.calls, rid(1)); !slices.Contains(got, rid(1)+" redo") {
		t.Fatalf("%v", got)
	}
}

func TestActiveRunThatDiedWithARunnerResultIsReportedAndClosedAsHubStoppedError(t *testing.T) {
	w := setup(t, meta(rid(1), withSlot(1), func(m *store.Meta) {
		m.State = lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}
		m.StartedAt = at(10)
		m.Pgid = new(4321)
	}))
	w.env.results[rid(1)] = true
	w.run(t)
	if !slices.Equal(w.env.killed, []int{4321}) || own(w.env.calls, rid(1))[0] != rid(1)+" removeOneOffs" {
		t.Fatalf("%v %v", w.env.killed, w.env.calls)
	}
	if len(w.env.reports) != 1 || w.env.reports[0].Verdict != lifecycle.VerdictError {
		t.Fatalf("reports %+v", w.env.reports)
	}
	m := w.read(t, rid(1))
	if m.Phase != lifecycle.Done || m.Env != lifecycle.EnvUp || m.Verdict != lifecycle.VerdictError || m.FailedStage != lifecycle.Testing || m.Pgid != nil {
		t.Fatalf("%+v", m)
	}
	if !strings.Contains(m.Notice, "hub stopped") {
		t.Fatalf("notice %q", m.Notice)
	}
}

func TestReportFailureOfADeadActiveRunMasksThePlantedSecretInTheNotice(t *testing.T) {
	w := setup(t, meta(rid(1), withSlot(1), func(m *store.Meta) {
		m.State = lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}
		m.StartedAt = at(10)
	}))
	w.env.results[rid(1)] = true
	w.env.reportErr = errorString("could not write the bundle: " + planted)
	w.run(t)
	m := w.read(t, rid(1))
	if m.Phase != lifecycle.Done || m.Verdict != lifecycle.VerdictError || strings.Contains(m.Notice, planted) || !strings.Contains(m.Notice, "report failed: could not write the bundle: ****") {
		t.Fatalf("%+v", m)
	}
}

func TestCleanupFailuresOfADeadActiveRunMaskThePlantedSecretInTheNotice(t *testing.T) {
	w := setup(t, meta(rid(1), withSlot(1), func(m *store.Meta) {
		m.State = lifecycle.State{Phase: lifecycle.Building, Env: lifecycle.EnvAbsent}
		m.StartedAt = at(10)
		m.Pgid = new(4321)
	}))
	w.env.killErr = errorString("kill denied for " + planted)
	w.env.removeErr = errorString("rm failed with " + planted)
	w.run(t)
	m := w.read(t, rid(1))
	if strings.Contains(m.Notice, planted) || !strings.Contains(m.Notice, "could not kill pgid 4321 (kill denied for ****)") || !strings.Contains(m.Notice, "could not remove one-off containers (rm failed with ****)") {
		t.Fatalf("notice %q", m.Notice)
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }

func TestActiveRunWithoutARunnerResultClosesWithoutAReport(t *testing.T) {
	w := setup(t, meta(rid(1), withSlot(1), func(m *store.Meta) {
		m.State = lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}
		m.StartedAt = at(10)
	}))
	w.run(t)
	if len(w.env.reports) != 0 {
		t.Fatalf("reports %d", len(w.env.reports))
	}
	if m := w.read(t, rid(1)); m.Phase != lifecycle.Done || m.Verdict != lifecycle.VerdictError {
		t.Fatalf("%+v", m.State)
	}
}

func TestGroupFromAnEarlierBootIsNotKilledAndTheNoticeSaysSo(t *testing.T) {
	w := setup(t, meta(rid(1), withSlot(1), func(m *store.Meta) {
		m.State = lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}
		m.StartedAt = "2026-10-11T23:00:00.000Z"
		m.Pgid = new(4321)
	}))
	w.run(t)
	m := w.read(t, rid(1))
	if len(w.env.killed) != 0 || len(w.env.reports) != 0 || m.Verdict != lifecycle.VerdictError || m.Env != lifecycle.EnvUp {
		t.Fatalf("%v %+v", w.env.killed, m)
	}
	if !strings.Contains(m.Notice, "earlier boot") {
		t.Fatalf("notice %q", m.Notice)
	}
}

func TestRunDiedWhileBuildingClosesWithoutEnvironmentAndTakesDownLeftovers(t *testing.T) {
	w := setup(t, meta(rid(1), withSlot(1), func(m *store.Meta) {
		m.State = lifecycle.State{Phase: lifecycle.Building, Env: lifecycle.EnvAbsent}
		m.StartedAt = at(10)
	}))
	w.docker.labelled[rid(1)] = Labelled{Networks: 1}
	w.run(t)
	m := w.read(t, rid(1))
	if m.Phase != lifecycle.Done || m.Env != lifecycle.EnvAbsent || m.Verdict != lifecycle.VerdictError || m.FailedStage != lifecycle.Building {
		t.Fatalf("%+v", m)
	}
	if !slices.Contains(w.env.calls, rid(1)+" closeWithoutEnv down=true") {
		t.Fatalf("%v", w.env.calls)
	}
}

func TestBrokenStateFileIsRebuiltWithQueueInSubmitOrderAndLeasesFromSlots(t *testing.T) {
	w := setup(t, preserved(1, 10), meta(rid(3), func(m *store.Meta) { m.CreatedAt = at(30) }), meta(rid(2), func(m *store.Meta) { m.CreatedAt = at(20) }))
	os.WriteFile(filepath.Join(w.store.Home(), "state.json"), []byte("{"), 0o600)
	w.run(t)
	read := w.store.ReadState()
	want := store.HubState{SchemaVersion: 2, Queue: []string{rid(2), rid(3)}, Leases: store.Leases{rid(1): 1}}
	if read.Broken != "" || !reflect.DeepEqual(read.State, want) {
		t.Fatalf("%+v", read)
	}
}

func TestSkippedRunsAreLeftForTheirOwnOperation(t *testing.T) {
	w := setup(t, meta(rid(1), withSlot(1), func(m *store.Meta) { m.State = lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp} }))
	w.hooks.Skip[rid(1)] = true
	w.run(t)
	if m := w.read(t, rid(1)); m.Phase != lifecycle.Testing || len(w.env.calls) != 0 {
		t.Fatalf("%+v %v", m.State, w.env.calls)
	}
}

func TestLabelLookupIsAskedOncePerProjectName(t *testing.T) {
	w := setup(t)
	w.docker.ls = []Project{{Name: "toy", Status: "running(1)"}}
	w.run(t)
	w.run(t)
	n := 0
	for _, c := range w.docker.calls {
		if c == "labelled toy" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("asked %d times", n)
	}
}

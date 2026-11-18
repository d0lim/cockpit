package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/wire"
)

var wireDetachMutations = []porttest.Mutation{
	{"the report keeps a record it could not move", "leave Keep unset in DefaultReport", "red: the takes under report/wire are deleted with the old bundle", "TestAFailedDetachKeepsEveryStoredTakeAndTheReportStillCompletes"},
	{"a failed detach does not stop the take", "return a failed status from TakeSuite when Detach fails", "red: the suite take is not stored", "TestATakeMadeAfterAFailedDetachIsCarriedIntoTheNewBundle"},
}

func TestWireDetachMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "wiredetach_test.go", wireDetachMutations)
}

func blockDetach(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(stagedRecord(dir), []byte("blocks the move"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := wire.OpenStore(dir).Detach(); err == nil {
		t.Fatal("the injected failure did not make Detach fail")
	}
}

func TestAFailedDetachKeepsEveryStoredTakeAndTheReportStillCompletes(t *testing.T) {
	w := wireWorld(t, nil)
	m, dir := w.reportedWire()
	if r := w.sched.Exec(m.ID, "health", tester); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	before := wireFiles(t, bundleRecord(dir))
	if len(before) != 4 {
		t.Fatalf("the record holds %d files before the report", len(before))
	}
	stale := filepath.Join(dir, report.DirReport, "stale.txt")
	if err := os.WriteFile(stale, []byte("from the old bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	blockDetach(t, dir)
	asked := len(w.fake.Wire.Taken())
	if _, err := w.hub.ops.Report(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	absent(t, stale)
	present(t, filepath.Join(dir, report.DirReport, report.FileSummary))
	after := wireFiles(t, bundleRecord(dir))
	if len(after) != len(before) {
		t.Fatalf("the record holds %d files, it held %d", len(after), len(before))
	}
	for name, text := range before {
		if after[name] != text {
			t.Errorf("%s changed", name)
		}
	}
	if status := summaryWire(t, dir); status.State != wire.StateTaken || status.Calls != 2 {
		t.Fatalf("summary.wire %+v", status)
	}
	if got := len(w.fake.Wire.Taken()); got != asked {
		t.Fatalf("a stored take was asked for again: %d after %d", got, asked)
	}
}

func TestATakeMadeAfterAFailedDetachIsCarriedIntoTheNewBundle(t *testing.T) {
	w := wireWorld(t, func(f *fakedocker.Fake) { f.Wire.Gone = true })
	m, dir := w.reportedWire()
	if status := summaryWire(t, dir); status.State != wire.StateLost {
		t.Fatalf("canary: summary.wire %+v", status)
	}
	w.fake.Wire.Gone = false
	blockDetach(t, dir)
	if _, err := w.hub.ops.Report(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	present(t, filepath.Join(bundleRecord(dir), wire.SuiteTake, wire.CallsFile))
	if status := summaryWire(t, dir); status.State != wire.StateTaken || status.Calls != 2 {
		t.Fatalf("summary.wire %+v", status)
	}
}

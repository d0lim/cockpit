package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/signal"
)

var restartMutations = []porttest.Mutation{
	{"reconcile re-arms a preserved run it sees up", "drop the done/up observed/up arm row from lifecycle.HookFor", "red: the stream holds no gap event after the restart", "TestANewHubOnTheSameHomeReArmsTheUpPreservedRunWithExactlyOneGapEventAndBeatsResume"},
	{"the re-arm after a restart records the gap once", "skip s.gap on a reconciled arm", "red: the stream holds no supervisor gap event", "TestANewHubOnTheSameHomeReArmsTheUpPreservedRunWithExactlyOneGapEventAndBeatsResume"},
}

func TestRestartMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "restart_test.go", restartMutations)
}

type runningProbe struct{ id string }

func (p runningProbe) ComposeLs(context.Context) ([]reconcile.Project, error) {
	return []reconcile.Project{{Name: p.id, Status: "running(4)"}}, nil
}

func (runningProbe) Info(context.Context) bool { return true }

func (runningProbe) Labelled(context.Context, string) (reconcile.Labelled, error) {
	return reconcile.Labelled{Containers: 4}, nil
}

func gapEvents(t *testing.T, s *signal.Stream) []contract.Signal {
	t.Helper()
	resp, err := s.Read(signal.ReadRequest{Limit: 5000})
	if err != nil {
		t.Fatal(err)
	}
	var out []contract.Signal
	for _, sig := range resp.Signals {
		if sig.Collector == collect.SupervisorCollector && sig.Attrs["status"] == "gap" {
			out = append(out, sig)
		}
	}
	return out
}

func TestANewHubOnTheSameHomeReArmsTheUpPreservedRunWithExactlyOneGapEventAndBeatsResume(t *testing.T) {
	w := collectingWorld(t)
	m := w.submit()
	if m.Phase != lifecycle.Done || m.Env != lifecycle.EnvUp {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	dir, _ := w.hub.Store().RunDir(m.ID)
	alive := filepath.Join(dir, "collect-alive.json")
	var heartbeat []byte
	waitFor(t, "a collector heartbeat on the preserved run", func() bool {
		data, err := os.ReadFile(alive)
		heartbeat = data
		return err == nil && len(data) > 0
	})
	last, ok, err := collect.FileAlive{Dir: dir}.Load()
	if err != nil || !ok {
		t.Fatalf("heartbeat %v %v", ok, err)
	}
	w.sched.Dispose()
	w.hub.Close()
	if err := os.WriteFile(alive, heartbeat, 0o600); err != nil {
		t.Fatal(err)
	}
	opts := w.opts
	opts.Probe = runningProbe{id: m.ID}
	sched, hub, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sched.Dispose(); hub.Close() })
	stream, err := hub.Streams().Get(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(gapEvents(t, stream)); n != 0 {
		t.Fatalf("%d gap events before the restart", n)
	}
	sched.Reconcile()
	sched.Idle()
	if got, _ := hub.Store().MustRead(m.ID); got.Phase != lifecycle.Done || got.Env != lifecycle.EnvUp {
		t.Fatalf("reconcile moved the run to %+v", got.State)
	}
	gaps := gapEvents(t, stream)
	if len(gaps) != 1 || gaps[0].Attrs["from"] != last.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("gap events %+v, want one from %s", gaps, last)
	}
	waitFor(t, "a beat after the restart", func() bool {
		resp, err := stream.Read(signal.ReadRequest{After: gaps[0].Seq, Limit: 5000})
		if err != nil {
			return false
		}
		for _, sig := range resp.Signals {
			if sig.Collector == "beat" {
				return true
			}
		}
		return false
	})
	states := hub.Collectors().Status(m.ID)
	if len(states) != 1 || states[0].State != collect.StateArmed {
		t.Fatalf("collector states after the restart %+v", states)
	}
	sched.Reconcile()
	sched.Idle()
	if n := len(gapEvents(t, stream)); n != 1 {
		t.Fatalf("a second reconcile added gap events: %d", n)
	}
}

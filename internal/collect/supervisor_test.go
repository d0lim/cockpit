package collect_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/collect/collecttest"
	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var supervisorMutations = []porttest.Mutation{
	{"drain cancels in-flight calls before compose runs", "drop g.cancel() in Drain", "red: Drain returns only after the drain wait while the call is still running", "TestPeriodicCollectorRunsWhileUpStopsWithoutFalseFailuresAtStopAndRunsAgainAfterResume"},
	{"canceled runs are not failures", "record the outcome even when the generation is canceled", "red: the canceled run leaves an unread record", "TestPeriodicCollectorRunsWhileUpStopsWithoutFalseFailuresAtStopAndRunsAgainAfterResume"},
	{"slow ticks do not overlap and merge into one follow-up", "launch the runner in a goroutine without waiting for it", "red: two runs are active at once or missed ticks burst", "TestSlowTickNeverOverlapsAndRunsOnceMoreRightAfterItEnds"},
	{"the periodic budget is min(interval, cap)", "pass MaxBudget as the deadline", "red: the deadline is 1m for a 50ms interval", "TestSlowTickNeverOverlapsAndRunsOnceMoreRightAfterItEnds"},
	{"a hub restart re-arms with one gap event", "skip gap on CauseReconciled", "red: no gap event after the restart", "TestRearmAfterAHubRestartLeavesExactlyOneGapEventAndACleanDrainLeavesNone"},
	{"a clean drain clears the heartbeat", "skip Alive.Clear in Drain", "red: a reconciled arm after a drain emits a gap", "TestRearmAfterAHubRestartLeavesExactlyOneGapEventAndACleanDrainLeavesNone"},
	{"observers run under the same supervision", "skip starting observers in Arm", "red: the observer never starts", "TestObserversRunWhileArmedStopAtTheDrainAndTheirFailuresFoldIntoUnreadRecords"},
	{"observer failures are unread records", "drop the fold in observe", "red: the failing observer leaves no unread record", "TestObserversRunWhileArmedStopAtTheDrainAndTheirFailuresFoldIntoUnreadRecords"},
	{"hooks dispatch arm and drain", "make Fire return nil for every hook", "red: the collector never runs after a booted hook", "TestFireArmsOnArmHooksAndDrainsOnDrainHooks"},
}

type gate struct {
	mu       sync.Mutex
	entered  chan struct{}
	exits    []time.Time
	canceled int
	calls    int
}

func (g *gate) Call(ctx context.Context, _ string, _ []json.RawMessage) (any, error) {
	g.mu.Lock()
	g.calls++
	g.mu.Unlock()
	select {
	case g.entered <- struct{}{}:
	default:
	}
	select {
	case <-time.After(150 * time.Millisecond):
	case <-ctx.Done():
		g.mu.Lock()
		g.canceled++
		g.mu.Unlock()
	}
	g.mu.Lock()
	g.exits = append(g.exits, time.Now())
	g.mu.Unlock()
	return "v", nil
}

func (g *gate) snapshot() (int, int, []time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls, g.canceled, append([]time.Time(nil), g.exits...)
}

func TestPeriodicCollectorRunsWhileUpStopsWithoutFalseFailuresAtStopAndRunsAgainAfterResume(t *testing.T) {
	g := &gate{entered: make(chan struct{}, 1)}
	code := collecttest.Bundle(t, "toy-db", `await ctx.state.get("cursor");`)
	s := newSupervisor(nil, collect.Options{DrainWait: 3 * time.Second})
	register(t, s, collect.RunConfig{
		Collectors: []collect.Collector{{ID: "toy-db", Code: code, Interval: time.Second, Allowed: []string{ports.PortState}}},
		Ports:      func(string) map[string]ports.Handler { return map[string]ports.Handler{ports.PortState: g} },
	})
	arm(t, s, lifecycle.CauseBooted)
	eventually(t, 5*time.Second, "two collector runs", func() bool { calls, _, _ := g.snapshot(); return calls >= 2 })
	<-g.entered
	if err := s.Drain(context.Background(), supRun, lifecycle.CauseStopping); err != nil {
		t.Fatal(err)
	}
	composeStop := time.Now()
	calls, canceled, exits := g.snapshot()
	if canceled != 1 || len(exits) != calls || exits[len(exits)-1].After(composeStop) {
		t.Fatalf("calls=%d canceled=%d exits=%v stop=%v", calls, canceled, exits, composeStop)
	}
	if u := s.Unread(supRun); len(u) != 0 {
		t.Fatalf("false failures: %+v", u)
	}
	if st := s.Status(supRun); st[0].State != collect.StateStopped || st[0].Failures != 0 || st[0].Line != "toy-db: stopped" {
		t.Fatalf("%+v", st)
	}
	time.Sleep(1200 * time.Millisecond)
	if after, _, _ := g.snapshot(); after != calls {
		t.Fatalf("calls continued after the drain: %d then %d", calls, after)
	}
	arm(t, s, lifecycle.CauseResumed)
	eventually(t, 5*time.Second, "a run after resume", func() bool { n, _, _ := g.snapshot(); return n > calls })
	if st := s.Status(supRun); st[0].State != collect.StateArmed || st[0].LastSuccess == "" {
		t.Fatalf("%+v", st)
	}
}

func TestSlowTickNeverOverlapsAndRunsOnceMoreRightAfterItEnds(t *testing.T) {
	f := &fakeRunner{fn: func(ctx context.Context, _ worker.Spec) (worker.Outcome, error) {
		return waitOrCancel(ctx, 70*time.Millisecond), nil
	}}
	s := newSupervisor(f, collect.Options{})
	register(t, s, collect.RunConfig{Collectors: []collect.Collector{{ID: "slow", Interval: 50 * time.Millisecond}}})
	arm(t, s, lifecycle.CauseBooted)
	time.Sleep(400 * time.Millisecond)
	_ = s.Drain(context.Background(), supRun, lifecycle.CauseStopping)
	runs := f.records("slow")
	if f.peak() != 1 || len(runs) < 4 || len(runs) > 7 {
		t.Fatalf("peak=%d runs=%d", f.peak(), len(runs))
	}
	for i := 1; i < len(runs); i++ {
		if gap := runs[i].start.Sub(runs[i-1].end); gap < 0 || gap > 25*time.Millisecond {
			t.Fatalf("run %d started %v after the previous one ended", i, gap)
		}
	}
	if runs[0].deadline != 50*time.Millisecond {
		t.Fatalf("deadline %v", runs[0].deadline)
	}
	capped := &fakeRunner{fn: f.fn}
	s2 := newSupervisor(capped, collect.Options{MaxBudget: 30 * time.Millisecond})
	register(t, s2, collect.RunConfig{RunID: "run-2", Collectors: []collect.Collector{{ID: "slow", Interval: time.Second}}})
	if err := s2.Arm(context.Background(), "run-2", lifecycle.CauseBooted); err != nil {
		t.Fatal(err)
	}
	eventually(t, time.Second, "one capped run", func() bool { return len(capped.records("slow")) == 1 })
	if d := capped.records("slow")[0].deadline; d != 30*time.Millisecond {
		t.Fatalf("capped deadline %v", d)
	}
}

func TestRearmAfterAHubRestartLeavesExactlyOneGapEventAndACleanDrainLeavesNone(t *testing.T) {
	dir := t.TempDir()
	quick := &fakeRunner{fn: func(context.Context, worker.Spec) (worker.Outcome, error) {
		return worker.Outcome{Status: worker.StatusOK}, nil
	}}
	first := &freezableAlive{inner: collect.FileAlive{Dir: dir}}
	emit := &memEmitter{}
	cfg := collect.RunConfig{Collectors: []collect.Collector{{ID: "toy-db", Interval: 20 * time.Millisecond}}, Signals: emit}
	old := newSupervisor(quick, collect.Options{Heartbeat: 10 * time.Millisecond})
	cfg.Alive = first
	register(t, old, cfg)
	arm(t, old, lifecycle.CauseBooted)
	time.Sleep(50 * time.Millisecond)
	first.freeze()
	last, ok, err := first.Load()
	if err != nil || !ok {
		t.Fatalf("heartbeat: %v %v", ok, err)
	}
	if n := len(emit.byCollector(collect.SupervisorCollector)); n != 0 {
		t.Fatalf("a booted arm emitted %d gap events", n)
	}
	restarted := newSupervisor(quick, collect.Options{Heartbeat: 10 * time.Millisecond})
	cfg.Alive = collect.FileAlive{Dir: dir}
	register(t, restarted, cfg)
	arm(t, restarted, lifecycle.CauseReconciled)
	arm(t, restarted, lifecycle.CauseReconciled)
	gaps := emit.byCollector(collect.SupervisorCollector)
	if len(gaps) != 1 || gaps[0].Attrs["status"] != "gap" || gaps[0].Attrs["from"] != last.UTC().Format(time.RFC3339Nano) || gaps[0].Event == nil {
		t.Fatalf("%+v", gaps)
	}
	before := len(quick.records("toy-db"))
	eventually(t, time.Second, "runs after the re-arm", func() bool { return len(quick.records("toy-db")) > before+2 })
	if err := restarted.Drain(context.Background(), supRun, lifecycle.CauseStopping); err != nil {
		t.Fatal(err)
	}
	again := newSupervisor(quick, collect.Options{})
	register(t, again, cfg)
	arm(t, again, lifecycle.CauseReconciled)
	if n := len(emit.byCollector(collect.SupervisorCollector)); n != 1 {
		t.Fatalf("a reconciled arm after a clean drain emitted a gap: %d events", n)
	}
}

func TestFireArmsOnArmHooksAndDrainsOnDrainHooks(t *testing.T) {
	quick := &fakeRunner{fn: func(context.Context, worker.Spec) (worker.Outcome, error) {
		return worker.Outcome{Status: worker.StatusOK}, nil
	}}
	s := newSupervisor(quick, collect.Options{})
	register(t, s, collect.RunConfig{Collectors: []collect.Collector{{ID: "toy-db", Interval: 10 * time.Millisecond}}})
	before := &lifecycle.State{Phase: lifecycle.Booting, Env: lifecycle.EnvAbsent}
	after, err := lifecycle.Transition(before, lifecycle.Booted, lifecycle.Detail{})
	if err != nil {
		t.Fatal(err)
	}
	if err := collect.Fire(context.Background(), s, supRun, lifecycle.HookFor(before, lifecycle.Booted, lifecycle.Detail{}, after)); err != nil {
		t.Fatal(err)
	}
	eventually(t, time.Second, "runs after the booted hook", func() bool { return len(quick.records("toy-db")) >= 2 })
	next, err := lifecycle.Transition(&after, lifecycle.Tested, lifecycle.Detail{})
	if err != nil {
		t.Fatal(err)
	}
	if err := collect.Fire(context.Background(), s, supRun, lifecycle.HookFor(&after, lifecycle.Tested, lifecycle.Detail{}, next)); err != nil {
		t.Fatal(err)
	}
	if st := s.Status(supRun); st[0].State != collect.StateStopped {
		t.Fatalf("after the reporting hook: %+v", st)
	}
	n := len(quick.records("toy-db"))
	time.Sleep(50 * time.Millisecond)
	if len(quick.records("toy-db")) != n {
		t.Fatal("runs continued after the drain hook")
	}
}

type fakeObserver struct {
	mu      sync.Mutex
	name    string
	starts  int
	stops   int
	failure error
}

func (o *fakeObserver) Name() string { return o.name }

func (o *fakeObserver) Run(ctx context.Context) error {
	o.mu.Lock()
	o.starts++
	o.mu.Unlock()
	if o.failure != nil {
		return o.failure
	}
	<-ctx.Done()
	o.mu.Lock()
	o.stops++
	o.mu.Unlock()
	return nil
}

func (o *fakeObserver) counts() (int, int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.starts, o.stops
}

func TestObserversRunWhileArmedStopAtTheDrainAndTheirFailuresFoldIntoUnreadRecords(t *testing.T) {
	logs := &fakeObserver{name: "logs"}
	broken := &fakeObserver{name: "services", failure: errors.New("ps failed for " + supPlanted)}
	s := newSupervisor(&fakeRunner{}, collect.Options{})
	register(t, s, collect.RunConfig{Observers: []collect.Observer{logs, broken}})
	arm(t, s, lifecycle.CauseBooted)
	eventually(t, time.Second, "both observers started", func() bool { a, _ := logs.counts(); b, _ := broken.counts(); return a == 1 && b >= 1 })
	if err := s.Drain(context.Background(), supRun, lifecycle.CauseStopping); err != nil {
		t.Fatal(err)
	}
	if starts, stops := logs.counts(); starts != 1 || stops != 1 {
		t.Fatalf("logs observer starts=%d stops=%d", starts, stops)
	}
	u := s.Unread(supRun)
	if len(u) != 1 || u[0].Source != "observer:services" || u[0].Reason != "ps failed for ****" {
		t.Fatalf("%+v", u)
	}
}

func TestSupervisorMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "supervisor_test.go", supervisorMutations)
}

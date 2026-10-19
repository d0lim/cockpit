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
	{"the next tick waits for the interval from the run start", "compute the wait from the run end", "red: the capped collector's next tick is 10ms late", "TestSlowTickNeverOverlapsAndRunsOnceMoreRightAfterItEnds"},
	{"the periodic budget is min(interval, cap)", "pass MaxBudget as the deadline", "red: the deadline is 1m for a 50ms interval", "TestSlowTickNeverOverlapsAndRunsOnceMoreRightAfterItEnds"},
	{"a hub restart re-arms with one gap event", "skip gap on CauseReconciled", "red: no gap event after the restart", "TestRearmAfterAHubRestartLeavesExactlyOneGapEventAndACleanDrainLeavesNone"},
	{"a clean drain clears the heartbeat", "skip Alive.Clear in Drain", "red: a reconciled arm after a drain emits a gap", "TestRearmAfterAHubRestartLeavesExactlyOneGapEventAndACleanDrainLeavesNone"},
	{"observers run under the same supervision", "skip starting observers in Arm", "red: the observer never starts", "TestObserversRunWhileArmedStopAtTheDrainAndTheirFailuresFoldIntoUnreadRecords"},
	{"observer failures are unread records", "drop the fold in observe", "red: the failing observer leaves no unread record", "TestObserversRunWhileArmedStopAtTheDrainAndTheirFailuresFoldIntoUnreadRecords"},
	{"hooks dispatch arm and drain", "make Fire return nil for every hook", "red: the collector never runs after a booted hook", "TestFireArmsOnArmHooksAndDrainsOnDrainHooks"},
}

type gate struct {
	mu       sync.Mutex
	hold     int
	held     chan struct{}
	exits    []time.Time
	canceled int
	calls    int
}

func (g *gate) Call(ctx context.Context, _ string, _ []json.RawMessage) (any, error) {
	g.mu.Lock()
	g.calls++
	holding := g.calls == g.hold
	g.mu.Unlock()
	if holding {
		g.held <- struct{}{}
		<-ctx.Done()
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
	g := &gate{hold: 2, held: make(chan struct{}, 1)}
	code := collecttest.Bundle(t, "toy-db", `await ctx.state.get("cursor");`)
	clock := collecttest.NewClock(epoch)
	s := newSupervisor(nil, collect.Options{DrainWait: 10 * time.Second, Now: clock.Now, Sleep: clock.Sleep})
	register(t, s, collect.RunConfig{
		Collectors: []collect.Collector{{ID: "toy-db", Code: code, Interval: time.Hour, Allowed: []string{ports.PortState}}},
		Ports:      func(string) map[string]ports.Handler { return map[string]ports.Handler{ports.PortState: g} },
	})
	arm(t, s, lifecycle.CauseBooted)
	clock.AwaitSleepers(t, 1)
	if calls, _, _ := g.snapshot(); calls != 1 {
		t.Fatalf("the first tick made %d calls", calls)
	}
	clock.Next()
	<-g.held
	if err := s.Drain(context.Background(), supRun, lifecycle.CauseStopping); err != nil {
		t.Fatal(err)
	}
	composeStop := time.Now()
	calls, canceled, exits := g.snapshot()
	if calls != 2 || canceled != 1 || len(exits) != calls || exits[len(exits)-1].After(composeStop) {
		t.Fatalf("calls=%d canceled=%d exits=%v stop=%v", calls, canceled, exits, composeStop)
	}
	if u := s.Unread(supRun); len(u) != 0 {
		t.Fatalf("false failures: %+v", u)
	}
	if st := s.Status(supRun); st[0].State != collect.StateStopped || st[0].Failures != 0 || st[0].Line != "toy-db: stopped" {
		t.Fatalf("%+v", st)
	}
	if n := clock.Sleepers(); n != 0 || clock.Next() {
		t.Fatalf("%d goroutines still wait for a tick after the drain", n)
	}
	if after, _, _ := g.snapshot(); after != calls {
		t.Fatalf("calls continued after the drain: %d then %d", calls, after)
	}
	arm(t, s, lifecycle.CauseResumed)
	clock.AwaitSleepers(t, 1)
	if n, _, _ := g.snapshot(); n != calls+1 {
		t.Fatalf("the resume tick made %d calls in all, want %d", n, calls+1)
	}
	if st := s.Status(supRun); st[0].State != collect.StateArmed || st[0].LastSuccess != epoch.Add(time.Hour).Format(time.RFC3339Nano) {
		t.Fatalf("%+v", st)
	}
}

func TestSlowTickNeverOverlapsAndRunsOnceMoreRightAfterItEnds(t *testing.T) {
	clock := collecttest.NewClock(epoch)
	held := make(chan struct{}, 1)
	var n int
	f := &fakeRunner{now: clock.Now}
	f.fn = func(ctx context.Context, _ worker.Spec) (worker.Outcome, error) {
		clock.Advance(70 * time.Millisecond)
		if n++; n == 5 {
			held <- struct{}{}
			<-ctx.Done()
			return worker.Outcome{Status: worker.StatusKilled, Kill: worker.KillCanceled}, nil
		}
		return worker.Outcome{Status: worker.StatusOK}, nil
	}
	s := newSupervisor(f, collect.Options{Now: clock.Now, Sleep: clock.Sleep})
	register(t, s, collect.RunConfig{Collectors: []collect.Collector{{ID: "slow", Interval: 50 * time.Millisecond}}})
	arm(t, s, lifecycle.CauseBooted)
	<-held
	if err := s.Drain(context.Background(), supRun, lifecycle.CauseStopping); err != nil {
		t.Fatal(err)
	}
	runs := f.records("slow")
	if f.peak() != 1 || len(runs) != 5 || clock.Sleepers() != 0 {
		t.Fatalf("peak=%d runs=%d sleepers=%d", f.peak(), len(runs), clock.Sleepers())
	}
	for i := 1; i < len(runs); i++ {
		if !runs[i].start.Equal(runs[i-1].end) {
			t.Fatalf("run %d started %v after the previous one ended", i, runs[i].start.Sub(runs[i-1].end))
		}
	}
	if runs[0].deadline != 50*time.Millisecond {
		t.Fatalf("deadline %v", runs[0].deadline)
	}
	clock2 := collecttest.NewClock(epoch)
	capped := &fakeRunner{now: clock2.Now, fn: func(context.Context, worker.Spec) (worker.Outcome, error) {
		clock2.Advance(10 * time.Millisecond)
		return worker.Outcome{Status: worker.StatusOK}, nil
	}}
	s2 := newSupervisor(capped, collect.Options{MaxBudget: 30 * time.Millisecond, Now: clock2.Now, Sleep: clock2.Sleep})
	register(t, s2, collect.RunConfig{RunID: "run-2", Collectors: []collect.Collector{{ID: "slow", Interval: time.Second}}})
	if err := s2.Arm(context.Background(), "run-2", lifecycle.CauseBooted); err != nil {
		t.Fatal(err)
	}
	clock2.AwaitSleepers(t, 1)
	if recs := capped.records("slow"); len(recs) != 1 || recs[0].deadline != 30*time.Millisecond {
		t.Fatalf("capped runs %+v", recs)
	}
	if due, _ := clock2.Due(); !due.Equal(epoch.Add(time.Second)) {
		t.Fatalf("canary: a fast run waits for the interval from its start, next tick at %v", due.Sub(epoch))
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
	clock := collecttest.NewClock(epoch)
	quick := &fakeRunner{fn: func(context.Context, worker.Spec) (worker.Outcome, error) {
		return worker.Outcome{Status: worker.StatusOK}, nil
	}}
	s := newSupervisor(quick, collect.Options{Now: clock.Now, Sleep: clock.Sleep})
	register(t, s, collect.RunConfig{Collectors: []collect.Collector{{ID: "toy-db", Interval: 10 * time.Millisecond}}})
	before := &lifecycle.State{Phase: lifecycle.Booting, Env: lifecycle.EnvAbsent}
	after, err := lifecycle.Transition(before, lifecycle.Booted, lifecycle.Detail{})
	if err != nil {
		t.Fatal(err)
	}
	if err := collect.Fire(context.Background(), s, supRun, lifecycle.HookFor(before, lifecycle.Booted, lifecycle.Detail{}, after)); err != nil {
		t.Fatal(err)
	}
	clock.RunUntil(t, epoch.Add(10*time.Millisecond), 1)
	if n := len(quick.records("toy-db")); n != 2 {
		t.Fatalf("runs after the booted hook and one interval: %d", n)
	}
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
	if clock.Sleepers() != 0 || clock.Next() || len(quick.records("toy-db")) != 2 {
		t.Fatalf("a tick is still scheduled after the drain hook: sleepers=%d runs=%d", clock.Sleepers(), len(quick.records("toy-db")))
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

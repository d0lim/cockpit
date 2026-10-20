package pipeline

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/collect/collecttest"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var sqlPoolMutations = []porttest.Mutation{
	{"stop closes the run's database pool before compose stop", "delete the release call for drain hooks in Ops.Transition", "red: the reader connection is still open when compose stop runs", "TestStopAndDestroyCloseTheRunsDatabasePoolBeforeTheEnvironmentGoesDownAndResumeReopensIt"},
	{"destroy closes the run's database pool before compose down", "release only on the stopping cause", "red: the reader connection is still open when compose down runs", "TestStopAndDestroyCloseTheRunsDatabasePoolBeforeTheEnvironmentGoesDownAndResumeReopensIt"},
}

func TestSQLPoolMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "sqlpool_test.go", sqlPoolMutations)
}

const probeCollector = `import { type CollectCtx, collector } from "cockpit:collect";

export default collector({
	id: "probe",
	intervalMs: 1000,
	async run(ctx: CollectCtx) {
		await ctx.sql.query("main", "SELECT 1");
	},
});
`

type verbProbe struct {
	mu   sync.Mutex
	db   *livePostgres
	seen []string
	idle []bool
}

func (p *verbProbe) observe(verb string) {
	if verb != "compose stop" && verb != "compose down" || p.db.executes.Load() == 0 {
		return
	}
	idle := p.db.idleWithin(3 * time.Second)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen, p.idle = append(p.seen, verb), append(p.idle, idle)
}

func (p *verbProbe) results() ([]string, []bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.seen...), append([]bool(nil), p.idle...)
}

func TestStopAndDestroyCloseTheRunsDatabasePoolBeforeTheEnvironmentGoesDownAndResumeReopensIt(t *testing.T) {
	base := freeBlock(t)
	probe := &verbProbe{db: startLivePostgres(t, base+1)}
	w := newWorldWith(t, func(o *Options) {
		o.NoCollection, o.ScriptHost, o.Limits.Suite = false, collecttest.Command(), 10*time.Second
		o.Collect = collect.Options{DrainWait: 2 * time.Second, Heartbeat: 100 * time.Millisecond}
		o.Config.Limits.PortBase = base
	}, func(f *fakedocker.Fake) {
		f.Base, f.RunnerDelay, f.OnVerb = base, 2500*time.Millisecond, probe.observe
	}, func(dir string) {
		os.RemoveAll(filepath.Join(dir, "scripts", "collectors"))
		writeRepoFile(t, filepath.Join(dir, "scripts", "collectors", "probe.ts"), probeCollector)
	})
	w.base = base
	m := w.submit()
	if m.Verdict != lifecycle.VerdictPassed || m.Env != lifecycle.EnvUp {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	waitFor(t, "a collector query served over an open reader connection", func() bool {
		return probe.db.executes.Load() > 0 && probe.db.live.Load() > 0
	})
	actor := scheduler.Actor{By: "tester"}
	if r := w.sched.Stop(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	if r := w.sched.Resume(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	resumed := probe.db.executes.Load()
	waitFor(t, "a query after resume over a reopened reader connection", func() bool {
		return probe.db.executes.Load() > resumed && probe.db.live.Load() > 0
	})
	if r := w.sched.Destroy(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	verbs, idle := probe.results()
	if len(verbs) != 2 || verbs[0] != "compose stop" || verbs[1] != "compose down" {
		t.Fatalf("observed %v, want one compose stop and one compose down after a served query", verbs)
	}
	for i, v := range verbs {
		if !idle[i] {
			t.Errorf("%s ran while the run's reader connection was still open", v)
		}
	}
	if live := probe.db.live.Load(); live != 0 {
		t.Fatalf("%d connections stay open after destroy", live)
	}
}

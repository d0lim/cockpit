package pipeline

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/collect/collecttest"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
)

const execCollector = `import { type CollectCtx, collector } from "cockpit:collect";

export default collector({
	id: "dbsize",
	intervalMs: 1000,
	async run(ctx: CollectCtx) {
		await ctx.docker.exec("cache", ["valkey-cli", "DBSIZE"]);
		ctx.signal.metric({ name: "dbsize.calls", value: 1 });
	},
});
`

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestEveryHubLogLineOfARunWithACollectorExecGoesToTheInjectedLoggerAndNoneToTheProcessDefault(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	var fallback, hubOut lockedBuffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&fallback, nil)))
	w := newWorldWith(t, func(o *Options) {
		o.NoCollection, o.ScriptHost, o.Limits.Suite = false, collecttest.Command(), 10*time.Second
		o.Collect = collect.Options{DrainWait: 2 * time.Second, Heartbeat: 100 * time.Millisecond}
		o.Log = slog.New(slog.NewTextHandler(&hubOut, nil))
	}, func(f *fakedocker.Fake) {
		f.RunnerDelay = 1500 * time.Millisecond
	}, func(dir string) {
		os.RemoveAll(filepath.Join(dir, "scripts", "collectors"))
		writeRepoFile(t, filepath.Join(dir, "scripts", "collectors", "dbsize.ts"), execCollector)
	})
	m := w.submit()
	if m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	waitFor(t, "a collector exec audit line in the hub log", func() bool {
		return strings.Contains(hubOut.String(), `msg="docker exec" run=`+m.ID+` service=cache`)
	})
	actor := scheduler.Actor{By: "tester"}
	if r := w.sched.Stop(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	if r := w.sched.Resume(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	if r := w.sched.Destroy(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	if got := fallback.String(); got != "" {
		t.Fatalf("the process default logger got hub lines:\n%s", got)
	}
}

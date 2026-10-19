package observer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
	dockerport "github.com/d0lim/cockpit/internal/ports/docker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/signal"
)

const (
	planted = "PLANTED-observer-81c2"
	project = "cockpit-run-1"
	runID   = "run-1"
)

var logsMutations = []porttest.Mutation{
	{"log lines pass the run masker before they are cut", "skip l.Mask in line", "red: the prefix of the planted value survives the cut", "TestContainerLogLinesBecomeMaskedEventsWithPerContainerDropsAndNoHarnessOrForeignContainers"},
	{"harness containers are not followed", "follow every listed container in Containers", "red: docker logs runs for c-edge or c-probe", "TestContainerLogLinesBecomeMaskedEventsWithPerContainerDropsAndNoHarnessOrForeignContainers"},
	{"containers outside the run are not followed", "drop the project and run-id check in list", "red: docker logs runs for c-intruder", "TestContainerLogLinesBecomeMaskedEventsWithPerContainerDropsAndNoHarnessOrForeignContainers"},
	{"a reattached follow does not repeat lines", "skip the floor check in line", "red: the reattached stream stores the first line twice", "TestReattachedFollowStartsFromTheLastLineWithoutRepeatingIt"},
}

type fakeDocker struct {
	mu     sync.Mutex
	calls  [][]string
	ps     []string
	handle func(ctx context.Context, cmd compose.Command) (compose.Result, error)
}

func (f *fakeDocker) Run(ctx context.Context, cmd compose.Command) (compose.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), cmd.Args...))
	f.mu.Unlock()
	if cmd.Args[0] == "ps" {
		return compose.Result{Stdout: []byte(strings.Join(f.ps, "\n") + "\n")}, nil
	}
	return f.handle(ctx, cmd)
}

func (f *fakeDocker) logsFor() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int{}
	for _, c := range f.calls {
		if c[0] == "logs" {
			out[c[len(c)-1]]++
		}
	}
	return out
}

func psLine(id, service, proj, run, role string) string {
	return strings.Join([]string{id, proj + "-" + service + "-1", "running", "Up", proj, service, run, role}, "\t")
}

func store(t *testing.T, caps signal.Caps) *signal.Stream {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	s, err := signal.Open(filepath.Join(t.TempDir(), "run"), signal.Options{Masker: func() *mask.Masker { return m }, Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func events(t *testing.T, s *signal.Stream) []contract.Signal {
	t.Helper()
	resp, err := s.Read(signal.ReadRequest{Limit: 10000})
	if err != nil {
		t.Fatal(err)
	}
	return resp.Signals
}

func stamp(i int) string {
	return time.Date(2026, 10, 19, 1, 0, i, 0, time.UTC).Format(time.RFC3339Nano)
}

func TestContainerLogLinesBecomeMaskedEventsWithPerContainerDropsAndNoHarnessOrForeignContainers(t *testing.T) {
	maxLine := 64
	long := strings.Repeat("x", maxLine-5) + planted
	d := &fakeDocker{ps: []string{
		psLine("c-app", "app", project, runID, ""), psLine("c-db", "db", project, runID, ""),
		psLine("c-edge", "edge", project, runID, "edge"), psLine("c-probe", "probe", project, runID, "probe"),
		psLine("c-intruder", "app", "other", "run-9", ""),
	}}
	d.handle = func(ctx context.Context, cmd compose.Command) (compose.Result, error) {
		switch cmd.Args[len(cmd.Args)-1] {
		case "c-app":
			for i := range 12 {
				fmt.Fprintf(cmd.Stdout, "%s app line %d\n", stamp(i), i)
			}
			fmt.Fprintf(cmd.Stderr, "%s %s\n", stamp(20), long)
		case "c-db":
			for i := range 3 {
				fmt.Fprintf(cmd.Stderr, "%s db token=%s line %d\n", stamp(i), planted, i)
			}
		}
		<-ctx.Done()
		return compose.Result{}, ctx.Err()
	}
	dp, err := dockerport.New(dockerport.Options{Project: project, RunID: runID, Docker: d})
	if err != nil {
		t.Fatal(err)
	}
	s := store(t, signal.Caps{Tail: 1000, PerCollector: 1000, PerContainer: 5, CompactMin: 1 << 20})
	obs := &Logs{Lister: dp, Docker: d, Emit: s, Mask: func(v string) string { return strings.ReplaceAll(v, planted, "****") }, Poll: 20 * time.Millisecond, MaxLine: maxLine}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- obs.Run(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for len(events(t, s)) < 8 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(60 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	byContainer := map[string][]contract.Signal{}
	for _, sig := range events(t, s) {
		if sig.Collector != signal.LogCollector || sig.Kind != contract.SignalKindEvent || sig.TSSource != contract.TimeSourceContainer {
			t.Fatalf("%+v", sig)
		}
		c, _ := sig.Attrs[signal.ContainerAttr].(string)
		byContainer[c] = append(byContainer[c], sig)
	}
	if len(byContainer) != 2 || len(byContainer["db"]) != 3 || len(byContainer["app"]) != 5 {
		t.Fatalf("app=%d db=%d of %v", len(byContainer["app"]), len(byContainer["db"]), byContainer)
	}
	for _, sig := range byContainer["db"] {
		if strings.Contains(sig.Event.Body, planted) || !strings.Contains(sig.Event.Body, "token=****") {
			t.Fatalf("%q", sig.Event.Body)
		}
	}
	last := byContainer["app"][len(byContainer["app"])-1].Event.Body
	if strings.Contains(last, "PLANT") || !strings.HasSuffix(last, "****") || len(last) > maxLine {
		t.Fatalf("long line: %q", last)
	}
	resp, _ := s.Read(signal.ReadRequest{})
	if resp.Dropped[signal.ReasonContainerQuota] != 8 || len(resp.Dropped) != 1 {
		t.Fatalf("dropped %v", resp.Dropped)
	}
	followed := d.logsFor()
	if followed["c-app"] == 0 || followed["c-db"] == 0 || followed["c-edge"] != 0 || followed["c-probe"] != 0 || followed["c-intruder"] != 0 {
		t.Fatalf("followed %v", followed)
	}
}

func TestReattachedFollowStartsFromTheLastLineWithoutRepeatingIt(t *testing.T) {
	d := &fakeDocker{ps: []string{psLine("c-app", "app", project, runID, "")}}
	var mu sync.Mutex
	var sinces []string
	d.handle = func(ctx context.Context, cmd compose.Command) (compose.Result, error) {
		mu.Lock()
		sinces = append(sinces, cmd.Args[3])
		n := len(sinces)
		mu.Unlock()
		fmt.Fprintf(cmd.Stdout, "%s first\n", stamp(1))
		if n == 1 {
			return compose.Result{}, nil
		}
		fmt.Fprintf(cmd.Stdout, "%s second\n", stamp(2))
		<-ctx.Done()
		return compose.Result{}, ctx.Err()
	}
	dp, err := dockerport.New(dockerport.Options{Project: project, RunID: runID, Docker: d})
	if err != nil {
		t.Fatal(err)
	}
	s := store(t, signal.DefaultCaps())
	obs := &Logs{Lister: dp, Docker: d, Emit: s, Poll: 20 * time.Millisecond, Now: func() time.Time { return time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC) }}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- obs.Run(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for len(events(t, s)) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(40 * time.Millisecond)
	cancel()
	<-done
	var bodies []string
	for _, sig := range events(t, s) {
		bodies = append(bodies, sig.Event.Body)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(bodies, ",") != "first,second" || len(sinces) < 2 || sinces[0] != "--since=2026-10-19T00:00:00Z" || sinces[1] != "--since="+stamp(1) {
		t.Fatalf("bodies=%v sinces=%v", bodies, sinces)
	}
}

func TestLogsMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "logs_test.go", logsMutations)
}

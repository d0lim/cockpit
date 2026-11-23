package headless

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/hub/config"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/pipeline"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	hubstages "github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/runnerbin"
)

var parityExcluded = map[string]string{
	"runId":       "each run has its own id",
	"generatedAt": "each report has its own time",
}

func hubSummary(t *testing.T, repo string, f *fakedocker.Fake) map[string]any {
	t.Helper()
	limits := config.Defaults()
	opts := pipeline.Options{
		Home: t.TempDir(), Config: config.Config{AllowedRepositories: []string{repo}, Limits: limits, ReservedPorts: []config.PortRange{}},
		HubPort: 7710, DockerFor: f.For, Probe: fakedocker.Reconcile{},
		Ensure:       func(context.Context) (runnerbin.Image, error) { return fakeImage, nil },
		ListenCanary: canaryFor(f),
		Limits:       hubstages.Limits{Build: time.Minute, Boot: 2 * time.Second, Suite: 10 * time.Second, Call: time.Minute, Poll: 10 * time.Millisecond},
		NoCollection: true, PortProbe: func(int) bool { return true }, TickInterval: 20 * time.Millisecond,
	}
	sched, hub, err := pipeline.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sched.Dispose(); sched.Idle(); hub.Close() })
	res := sched.Submit(context.Background(), scheduler.SubmitInput{Worktree: repo, Submitter: "tester"})
	if res.Fail != nil {
		t.Fatalf("submit: %+v", res.Fail)
	}
	sched.Tick()
	sched.Idle()
	dir, err := hub.Store().RunDir(res.ID)
	if err != nil {
		t.Fatal(err)
	}
	return readJSON(t, filepath.Join(dir, "report", "summary.json"))
}

func withoutExcluded(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	removed := 0
	for field := range parityExcluded {
		if _, ok := doc[field]; ok {
			delete(doc, field)
			removed++
		}
	}
	if removed != len(parityExcluded) {
		t.Fatalf("%d of the %d excluded fields were present", removed, len(parityExcluded))
	}
	return doc
}

func TestTheSameFakeRunThroughTheHubAndThroughCockpitRunWritesTheSameSummaryButItsExcludedFields(t *testing.T) {
	failing, stop := failingSteps("expected status 200 but got 503")
	cases := []struct {
		name    string
		verdict string
		mutate  func(*fakedocker.Fake)
	}{
		{"passing", "passed", nil},
		{"failing step", "failed", func(f *fakedocker.Fake) { f.RunnerCode, f.Steps, f.StoppedAt = 1, failing, stop }},
	}
	compared := 0
	for _, c := range cases {
		repo := toyProject(t)
		hub := hubSummary(t, repo, newFake(c.mutate))
		out := filepath.Join(t.TempDir(), "bundle")
		code, text := runFake(t, repo, newFake(c.mutate), Options{Out: out})
		one := readJSON(t, filepath.Join(out, "summary.json"))
		if hub["verdict"] != c.verdict || one["verdict"] != c.verdict || code != map[string]int{"passed": 0, "failed": 1}[c.verdict] {
			t.Fatalf("%s: hub verdict %v, cockpit run verdict %v exit %d\n%s", c.name, hub["verdict"], one["verdict"], code, text)
		}
		hubStages, _ := hub["stages"].(map[string]any)
		oneStages, _ := one["stages"].(map[string]any)
		frozen := map[string]any{"status": "ok"}
		if !reflect.DeepEqual(hubStages["signals"], frozen) || !reflect.DeepEqual(oneStages["signals"], frozen) {
			t.Fatalf("%s: canary: both must freeze their signal stream: hub %v, cockpit run %v", c.name, hubStages["signals"], oneStages["signals"])
		}
		hub, one = withoutExcluded(t, hub), withoutExcluded(t, one)
		for key := range hub {
			if !reflect.DeepEqual(hub[key], one[key]) {
				t.Errorf("%s: %s differs: hub %v, cockpit run %v", c.name, key, hub[key], one[key])
			}
		}
		if len(hub) != len(one) || len(hub) < 20 {
			t.Errorf("%s: the hub summary has %d fields after the exclusions %v, cockpit run's %d", c.name, len(hub), parityExcluded, len(one))
		}
		compared++
	}
	if compared != 2 {
		t.Fatalf("%d of 2 cases compared", compared)
	}
}

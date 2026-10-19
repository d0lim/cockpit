package ticker

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var destroyRaceMutations = []porttest.Mutation{
	{"destroy follows the state its event landed on", "pick the queued branch from the meta read before the event", "red: the destroy answers success without cancelling the pipeline or destroying the environment", "TestADestroyWhoseRunStartsBetweenTheReadAndTheEventCancelsThePipelineAndDestroysTheEnvironment"},
}

func TestDestroyRaceMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "destroyrace_test.go", destroyRaceMutations)
}

func TestADestroyWhoseRunStartsBetweenTheReadAndTheEventCancelsThePipelineAndDestroysTheEnvironment(t *testing.T) {
	env := &quietEnv{}
	var started, cancelled atomic.Bool
	c, s := core(t, &gateDocker{}, env, func(d *Deps) {
		d.Pipeline = func(ctx context.Context, _ string) error {
			started.Store(true)
			<-ctx.Done()
			cancelled.Store(true)
			return nil
		}
	})
	c.afterRead = func() {
		c.afterRead = nil
		c.Tick()
	}
	r := c.DestroyRun(queuedID, lifecycle.DestroyPerson)
	if r.Fail != nil {
		t.Fatal(r.Fail)
	}
	m, _ := s.MustRead(queuedID)
	if !started.Load() || !cancelled.Load() || env.destroyed.Load() != 1 {
		t.Fatalf("pipeline started %v cancelled %v, env destroyed %d times; state %+v", started.Load(), cancelled.Load(), env.destroyed.Load(), m.State)
	}
	if m.FinishedAt != "" || m.Phase != lifecycle.Destroying {
		t.Fatalf("the destroy closed the run as if it were queued: %+v finished %q", m.State, m.FinishedAt)
	}
}

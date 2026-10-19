package reconcile

import (
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var hostAPIMutations = []porttest.Mutation{
	{"a queued run pinned to an unserved host API is refused at start", "drop the Serves branch in reconcileRun", "red: the unserved run stays queued", "TestQueuedRunPinnedToAnUnservedHostAPIIsRefusedAndLeavesTheQueue"},
	{"the refusal notice carries the served set", "write a notice without HostAPINotice", "red: the notice lacks the served set", "TestQueuedRunPinnedToAnUnservedHostAPIIsRefusedAndLeavesTheQueue"},
}

func TestQueuedRunPinnedToAnUnservedHostAPIIsRefusedAndLeavesTheQueue(t *testing.T) {
	w := setup(t, meta(rid(1), func(m *store.Meta) { m.HostAPI = 2 }), meta(rid(2), func(m *store.Meta) { m.CreatedAt = at(1) }))
	w.run(t)
	m := w.read(t, rid(1))
	if m.Phase != lifecycle.Done || m.Env != lifecycle.EnvAbsent || m.Verdict != lifecycle.VerdictError {
		t.Fatalf("%+v", m.State)
	}
	for _, want := range []string{"unknown host API version 2", "serves host API 1", "cockpit v0.3.0"} {
		if !strings.Contains(m.Notice, want) {
			t.Fatalf("notice %q lacks %q", m.Notice, want)
		}
	}
	queue := w.store.ReadState().State.Queue
	if slices.Contains(queue, rid(1)) || len(own(w.env.calls, rid(1))) != 0 {
		t.Fatalf("%v %v", queue, w.env.calls)
	}
	if served := w.read(t, rid(2)); served.Phase != lifecycle.Queued || !slices.Contains(queue, rid(2)) {
		t.Fatalf("canary: the served run left the queue: %+v %v", served.State, queue)
	}
}

func TestHostAPIMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "hostapi_test.go", hostAPIMutations)
}

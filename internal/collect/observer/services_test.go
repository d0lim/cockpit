package observer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	dockerport "github.com/d0lim/cockpit/internal/ports/docker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/signal"
)

var servicesMutations = []porttest.Mutation{
	{"service states are emitted only when they change", "drop the unchanged check in note", "red: every poll emits a revision", "TestServiceStatesAreEmittedAsRevisionsOnlyWhenTheyChangeAndGoneServicesBecomeAbsent"},
	{"a service that disappears becomes absent", "skip the absent pass in Run", "red: the gone worker keeps its last state", "TestServiceStatesAreEmittedAsRevisionsOnlyWhenTheyChangeAndGoneServicesBecomeAbsent"},
}

type psScript struct {
	mu    sync.Mutex
	steps [][]dockerport.ServiceState
	calls int
}

func (p *psScript) PS(context.Context) ([]dockerport.ServiceState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := min(p.calls, len(p.steps)-1)
	p.calls++
	return p.steps[i], nil
}

func (p *psScript) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestServiceStatesAreEmittedAsRevisionsOnlyWhenTheyChangeAndGoneServicesBecomeAbsent(t *testing.T) {
	src := &psScript{steps: [][]dockerport.ServiceState{
		{{Service: "app", State: "running", Health: "starting"}, {Service: "worker", State: "running"}},
		{{Service: "app", State: "running", Health: "starting"}, {Service: "worker", State: "running"}},
		{{Service: "app", State: "running", Health: "healthy"}},
	}}
	s := store(t, signal.DefaultCaps())
	obs := &Services{Source: src, Emit: s, Poll: 10 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- obs.Run(ctx) }()
	waitFor(t, "six ps polls", func() bool { return src.count() >= 6 })
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	resp, err := s.Read(signal.ReadRequest{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	latest := map[string]contract.Signal{}
	for _, sig := range resp.Signals {
		if sig.Collector != ServicesCollector || sig.Attrs["table"] != ServicesCollector {
			t.Fatalf("%+v", sig)
		}
		latest[sig.ID] = sig
	}
	app, worker := latest["service-app"], latest["service-worker"]
	if len(latest) != 2 || app.Rev != 2 || app.Attrs["health"] != "healthy" || app.Attrs["tone"] != string(contract.ToneOK) {
		t.Fatalf("app %+v", app)
	}
	if worker.Rev != 2 || worker.Attrs["status"] != "absent" || worker.Attrs["tone"] != string(contract.ToneIdle) {
		t.Fatalf("worker %+v", worker)
	}
	if revs, n := s.Revisions(ServicesCollector, contract.SignalKindEvent, "service-app"); n != 2 || revs < 0 {
		t.Fatalf("app emitted %d times over %d polls", n, src.count())
	}
}

func TestServicesMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "services_test.go", servicesMutations)
}

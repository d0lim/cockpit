package pipeline

import (
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
)

func TestPipelineEventsOpenWithStartCloseWithExitAndServicesCarryTheDeclaredPorts(t *testing.T) {
	w := newWorld(t, nil, nil)
	m := w.submit()
	w.mu.Lock()
	phases := append([]Event(nil), w.phases...)
	w.mu.Unlock()
	if len(phases) < 3 || phases[0].Phase != PhaseStart || phases[len(phases)-1].Phase != PhaseExit || *phases[len(phases)-1].Code != 0 {
		t.Fatalf("first %+v last %+v of %d", phases[0], phases[len(phases)-1], len(phases))
	}
	for _, e := range phases[1 : len(phases)-1] {
		if e.Phase != PhaseOut || e.Run != m.ID {
			t.Fatalf("middle event %+v", e)
		}
	}
	services := w.hub.Services(m)
	if len(services) != 3 || services[0].Name != "app" || services[0].HostPort != w.base || services[0].Open != contract.OpenBrowser || services[1].Open != contract.OpenCopy {
		t.Fatalf("services %+v", services)
	}
	failed := newWorld(t, func(f *fakedocker.Fake) { f.FailVerb = map[string]bool{"compose build": true} }, nil)
	failed.submit()
	failed.mu.Lock()
	last := failed.phases[len(failed.phases)-1]
	failed.mu.Unlock()
	if last.Phase != PhaseExit || last.Code == nil || *last.Code != 1 {
		t.Fatalf("failed run last event %+v", last)
	}
}

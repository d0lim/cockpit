package collect_test

import (
	"context"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var registerMutations = []porttest.Mutation{
	{"a run with collectors needs a port factory", "drop the port factory check from Register", "red: the config with a collector and no port factory registers", "TestARunOfObserversAloneRegistersWithoutAPortFactoryAndOneWithCollectorsDoesNot"},
	{"a run of observers alone needs no port factory", "require a port factory for every config in Register", "red: the observer-only config is refused", "TestARunOfObserversAloneRegistersWithoutAPortFactoryAndOneWithCollectorsDoesNot"},
}

func TestRegisterMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "register_test.go", registerMutations)
}

func TestARunOfObserversAloneRegistersWithoutAPortFactoryAndOneWithCollectorsDoesNot(t *testing.T) {
	s := newSupervisor(&fakeRunner{}, collect.Options{})
	if err := s.Register(collect.RunConfig{RunID: "with-collector", Collectors: []collect.Collector{{ID: "toy-db", Interval: time.Hour}}}); err == nil {
		t.Fatal("a config with a collector and no port factory registered")
	}
	logs := &fakeObserver{name: "logs"}
	if err := s.Register(collect.RunConfig{RunID: supRun, Observers: []collect.Observer{logs}}); err != nil {
		t.Fatalf("an observer-only config was refused: %v", err)
	}
	arm(t, s, lifecycle.CauseBooted)
	eventually(t, time.Second, "the observer started", func() bool { a, _ := logs.counts(); return a == 1 })
	if err := s.Drain(context.Background(), supRun, lifecycle.CauseReporting); err != nil {
		t.Fatal(err)
	}
	if starts, stops := logs.counts(); starts != 1 || stops != 1 {
		t.Fatalf("logs observer starts=%d stops=%d", starts, stops)
	}
}

package pipeline

import (
	"net"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
)

func servicesRows(view api.SignalsView) []contract.Signal {
	var out []contract.Signal
	for _, sig := range view.Signals {
		if sig.Collector == "services" {
			out = append(out, sig)
		}
	}
	return out
}

func (w *world) composePs() int {
	return len(slices.DeleteFunc(w.verbs(), func(v string) bool { return v != "compose ps" }))
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not happen within 15s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestAReArmAfterTheReportReemitsUnchangedServiceRowsWithoutNewRecordsSoTheFrozenWinnersStayInTheBundle(t *testing.T) {
	w := collectingWorld(t)
	m := w.submit()
	if m.Verdict != lifecycle.VerdictPassed || m.Env != lifecycle.EnvUp {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	srv := httptest.NewUnstartedServer(nil)
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	srv.Config.Handler = api.Handler(api.Deps{Port: port, Store: w.hub.Store(), Scheduler: w.sched, Signals: w.hub.Streams(), Collectors: w.hub.Collectors(), Mask: w.hub.Mask})
	srv.Start()
	defer srv.Close()
	reported := readSignals(t, srv.URL, m.ID)
	frozen := servicesRows(reported)
	if !reported.Frozen || len(frozen) != 3 || slices.ContainsFunc(frozen, func(s contract.Signal) bool { return s.Seq > reported.FrozenSeq }) {
		t.Fatalf("frozen %v at %d with %d service rows %+v", reported.Frozen, reported.FrozenSeq, len(frozen), frozen)
	}
	actor := scheduler.Actor{By: "tester"}
	if r := w.sched.Stop(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	polls := w.composePs()
	if r := w.sched.Resume(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	waitFor(t, "a second services poll after the re-arm", func() bool { return w.composePs() >= polls+2 })
	waitFor(t, "a beat after the re-arm", func() bool { return lastBeat(readSignals(t, srv.URL, m.ID)) > reported.FrozenSeq })
	after := servicesRows(readSignals(t, srv.URL, m.ID))
	if !slices.EqualFunc(after, frozen, func(a, b contract.Signal) bool { return a.Seq == b.Seq }) {
		t.Fatalf("service rows after the re-arm %v, want the frozen winners %v", seqsOf(after), seqsOf(frozen))
	}
	stream, err := w.hub.Streams().Get(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"app", "cache", "db"} {
		if _, n := stream.Revisions("services", contract.SignalKindEvent, "service-"+name); n != 1 {
			t.Errorf("service-%s has %d records, want 1", name, n)
		}
	}
}

func seqsOf(signals []contract.Signal) []int64 {
	out := []int64{}
	for _, sig := range signals {
		out = append(out, sig.Seq)
	}
	return out
}

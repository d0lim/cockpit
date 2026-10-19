package pipeline

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/report"
)

var apiReadMutations = []porttest.Mutation{
	{"the API, the collectors and the report share one stream per run", "register collectors on their own signal.OpenRun instead of the registry", "red: after resume the API read misses the beats the collectors append", "TestASignalAppendedByACollectorIsReadThroughTheAPIAndLandsInTheFrozenBundleCopy"},
}

func TestAPIReadMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "apiread_test.go", apiReadMutations)
}

func TestASignalAppendedByACollectorIsReadThroughTheAPIAndLandsInTheFrozenBundleCopy(t *testing.T) {
	w := collectingWorld(t)
	m := w.submit()
	if m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	srv := httptest.NewUnstartedServer(nil)
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	srv.Config.Handler = api.Handler(api.Deps{Port: port, Store: w.hub.Store(), Scheduler: w.sched, Signals: w.hub.Streams(), Collectors: w.hub.Collectors(), Mask: w.hub.Mask})
	srv.Start()
	defer srv.Close()
	dir, _ := w.hub.Store().RunDir(m.ID)
	seq := frozenLastBeat(t, fileText(t, filepath.Join(dir, report.DirReport, "signals", "stream.jsonl")))
	if seq == 0 {
		t.Fatal("the frozen copy holds no beat signal")
	}
	view := readSignals(t, srv.URL, m.ID)
	if !slices.ContainsFunc(view.Signals, func(sig contract.Signal) bool { return sig.Collector == "beat" && sig.Seq == seq }) {
		t.Fatalf("the API read lacks the frozen copy's last beat %d among %d signals", seq, len(view.Signals))
	}
	if len(view.Collectors) != 1 || view.Collectors[0].ID != "beat" || view.Collectors[0].Failures != 0 {
		t.Fatalf("collector states %+v", view.Collectors)
	}
	actor := scheduler.Actor{By: "tester"}
	if r := w.sched.Stop(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	if r := w.sched.Resume(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	time.Sleep(2500 * time.Millisecond)
	if r := w.sched.Stop(m.ID, actor); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	if later := lastBeat(readSignals(t, srv.URL, m.ID)); later <= seq {
		t.Fatalf("the API read shows no beat after resume (last %d, before %d)", later, seq)
	}
}

func readSignals(t *testing.T, base, id string) api.SignalsView {
	t.Helper()
	res, err := http.Get(base + "/api/runs/" + id + "/signals?limit=1000")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var view api.SignalsView
	if err := json.NewDecoder(res.Body).Decode(&view); err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %v", res.StatusCode, err)
	}
	return view
}

func frozenLastBeat(t *testing.T, text string) int64 {
	t.Helper()
	var seq int64
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		var sig contract.Signal
		if err := json.Unmarshal([]byte(line), &sig); err != nil {
			t.Fatalf("frozen line %q: %v", line, err)
		}
		if sig.Collector == "beat" {
			seq = max(seq, sig.Seq)
		}
	}
	return seq
}

func lastBeat(view api.SignalsView) int64 {
	var seq int64
	for _, sig := range view.Signals {
		if sig.Collector == "beat" {
			seq = max(seq, sig.Seq)
		}
	}
	return seq
}

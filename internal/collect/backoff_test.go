package collect_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var backoffMutations = []porttest.Mutation{
	{"same-reason failures fold into one counted record", "append a new ledger record for every failure", "red: the ledger holds one record per failure", "TestSameReasonFailuresFoldIntoOneCountedRecordAndBackOffAfterTheThreshold"},
	{"the next try backs off after N consecutive same-reason failures", "return 0 from backoff", "red: the failing collector runs every interval", "TestSameReasonFailuresFoldIntoOneCountedRecordAndBackOffAfterTheThreshold"},
	{"a different reason restarts the streak", "count consecutive failures regardless of the reason", "red: the alternating collector backs off", "TestSameReasonFailuresFoldIntoOneCountedRecordAndBackOffAfterTheThreshold"},
	{"the status line carries the failure count and reason, and stopped after a drain", "leave the state unchanged in Drain", "red: the line still says backing off after the drain", "TestStatusLineOfABackingOffCollectorShowsItsFailuresAndReasonAndSaysStoppedAfterADrain"},
	{"unread reasons pass the run masker", "store the failure text without mask", "red: the planted value shows in the unread record", "TestStatusLineOfABackingOffCollectorShowsItsFailuresAndReasonAndSaysStoppedAfterADrain"},
}

func failing(reason func(n int64) string) *fakeRunner {
	var n atomic.Int64
	return &fakeRunner{fn: func(context.Context, worker.Spec) (worker.Outcome, error) {
		return worker.Outcome{Status: worker.StatusFailed, Kind: "error", Failure: reason(n.Add(1))}, nil
	}}
}

func TestSameReasonFailuresFoldIntoOneCountedRecordAndBackOffAfterTheThreshold(t *testing.T) {
	f := failing(func(n int64) string {
		return "boom"
	})
	alt := failing(func(n int64) string { return []string{"left", "right"}[n%2] })
	s := newSupervisor(nil, collect.Options{FailAfter: 3, BackoffMax: 10 * time.Second, Runner: func(ctx context.Context, spec worker.Spec) (worker.Outcome, error) {
		if spec.Collector == "alt" {
			return alt.run(ctx, spec)
		}
		return f.run(ctx, spec)
	}})
	register(t, s, collect.RunConfig{Collectors: []collect.Collector{{ID: "same", Interval: 20 * time.Millisecond}, {ID: "alt", Interval: 20 * time.Millisecond}}})
	arm(t, s, lifecycle.CauseBooted)
	time.Sleep(300 * time.Millisecond)
	_ = s.Drain(context.Background(), supRun, lifecycle.CauseStopping)
	same, alternating := len(f.records("same")), len(alt.records("alt"))
	if same < 4 || same > 6 {
		t.Fatalf("the failing collector ran %d times in 300ms with backoff after 3", same)
	}
	if alternating < 10 {
		t.Fatalf("the alternating collector ran only %d times; a changing reason must not back off", alternating)
	}
	var sameRecs, altCount int
	for _, u := range s.Unread(supRun) {
		switch u.Collector {
		case "same":
			sameRecs++
			if u.Source != "collector:same" || u.Reason != "error: boom" || u.Count != same || u.First == "" || u.Last < u.First {
				t.Fatalf("%+v (ran %d)", u, same)
			}
		case "alt":
			altCount += u.Count
		}
	}
	if sameRecs != 1 || altCount != alternating {
		t.Fatalf("records for same=%d, alt counts=%d of %d runs", sameRecs, altCount, alternating)
	}
}

func TestStatusLineOfABackingOffCollectorShowsItsFailuresAndReasonAndSaysStoppedAfterADrain(t *testing.T) {
	f := failing(func(int64) string { return "token " + supPlanted + " rejected" })
	s := newSupervisor(f, collect.Options{FailAfter: 2, BackoffMax: time.Minute})
	register(t, s, collect.RunConfig{Collectors: []collect.Collector{{ID: "toy-db", Interval: 30 * time.Millisecond}, {ID: "once"}}})
	arm(t, s, lifecycle.CauseBooted)
	eventually(t, 2*time.Second, "backoff", func() bool { return s.Status(supRun)[0].State == collect.StateBackoff })
	st := s.Status(supRun)
	line := st[0].Line
	if st[0].Consecutive < 2 || st[0].Failures < 2 || st[0].NextAt == "" || !strings.Contains(line, "consecutive failures") || !strings.Contains(line, "error: token **** rejected") || strings.Contains(line, supPlanted) {
		t.Fatalf("%+v", st[0])
	}
	if st[1].State != collect.StateReportOnly || st[1].Failures != 0 {
		t.Fatalf("report-only collector: %+v", st[1])
	}
	u := s.Unread(supRun)
	if len(u) != 1 || strings.Contains(u[0].Reason, supPlanted) || !strings.Contains(u[0].Reason, "****") {
		t.Fatalf("%+v", u)
	}
	if err := s.Drain(context.Background(), supRun, lifecycle.CauseStopping); err != nil {
		t.Fatal(err)
	}
	for _, c := range s.Status(supRun) {
		if c.State != collect.StateStopped || c.Line != c.ID+": stopped" {
			t.Fatalf("%+v", c)
		}
	}
}

func TestBackoffMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "backoff_test.go", backoffMutations)
}

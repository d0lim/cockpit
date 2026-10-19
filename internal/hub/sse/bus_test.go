package sse

import (
	"slices"
	"testing"
	"time"
)

const runA = "run-20261019-010000-abc1234"
const runB = "run-20261019-010001-abc1234"

func kinds(events []Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = string(e.Kind) + "@" + e.Target
	}
	return out
}

func TestSubscribingReplaysTheTargetAndHubEventsInSeqOrderAndSkipsOtherRuns(t *testing.T) {
	b := New("1", 0, nil)
	b.Publish(KindJob, runA, map[string]any{"text": "one"})
	b.Publish(KindRuns, HubTarget, map[string]any{"list": 1})
	b.Publish(KindJob, runB, map[string]any{"text": "other"})
	b.Publish(KindLog, runA, map[string]any{"text": "two"})
	replay, ch, cancel := b.Subscribe(runA, 0)
	defer cancel()
	if got := kinds(replay); !slices.Equal(got, []string{"job@" + runA, "runs@hub", "log@" + runA}) {
		t.Fatalf("replay %v", got)
	}
	for i := 1; i < len(replay); i++ {
		if replay[i].Seq <= replay[i-1].Seq {
			t.Fatalf("replay is not in seq order: %v", replay)
		}
	}
	b.Publish(KindJob, runB, nil)
	b.Publish(KindServices, runA, map[string]any{"services": []string{}})
	if e := <-ch; e.Kind != KindServices || e.Target != runA {
		t.Fatalf("live %+v", e)
	}
	hub, _, cancelHub := b.Subscribe(HubTarget, 0)
	defer cancelHub()
	if got := kinds(hub); !slices.Equal(got, []string{"runs@hub"}) {
		t.Fatalf("hub replay %v", got)
	}
}

func TestReplayAfterASeqSendsOnlyLaterEventsAndSnapshotsKeepOnlyTheLast(t *testing.T) {
	b := New("1", 0, nil)
	first := b.Publish(KindJob, runA, nil)
	b.Publish(KindRuns, HubTarget, map[string]any{"n": 1})
	b.Publish(KindRuns, HubTarget, map[string]any{"n": 2})
	b.Publish(KindJob, runA, nil)
	replay, _, cancel := b.Subscribe(runA, first.Seq)
	defer cancel()
	if got := kinds(replay); !slices.Equal(got, []string{"runs@hub", "job@" + runA}) {
		t.Fatalf("replay %v", got)
	}
	if replay[0].Body.(map[string]any)["n"] != 2 {
		t.Fatalf("the snapshot is not the last one: %+v", replay[0])
	}
}

func TestTheBufferKeepsTheCapacityPerTargetAndKind(t *testing.T) {
	b := New("1", 3, nil)
	for range 5 {
		b.Publish(KindLog, runA, nil)
	}
	b.Publish(KindJob, runA, nil)
	got := b.Recent(runA)
	if len(got) != 4 || got[0].Seq != 3 || got[3].Kind != KindJob {
		t.Fatalf("recent %v", kinds(got))
	}
}

func TestASlowSubscriberIsDroppedAndTheBusKeepsPublishing(t *testing.T) {
	b := New("1", 0, nil)
	_, ch, cancel := b.Subscribe(runA, 0)
	defer cancel()
	done := make(chan struct{})
	go func() {
		for range subscriberQueue + 10 {
			b.Publish(KindLog, runA, nil)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("publish blocked on a slow subscriber")
	}
	n := 0
	for range ch {
		n++
	}
	if n != subscriberQueue || b.Audience(runA) != 0 {
		t.Fatalf("received %d, audience %d", n, b.Audience(runA))
	}
}

func TestAudienceCountsTargetSubscriptionsAndAnnouncesChanges(t *testing.T) {
	b := New("1", 0, nil)
	var seen []int
	b.OnAudience(func(target string, n int) {
		if target == runA {
			seen = append(seen, n)
		}
	})
	_, _, c1 := b.Subscribe(runA, 0)
	_, _, c2 := b.Subscribe(runA, 0)
	c1()
	c1()
	c2()
	if !slices.Equal(seen, []int{1, 2, 1, 0}) {
		t.Fatalf("announcements %v", seen)
	}
}

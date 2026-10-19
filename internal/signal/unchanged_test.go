package signal

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var unchangedMutations = []porttest.Mutation{
	{"an unchanged re-emission is a no-op", "drop the unchangedWinner check in Append", "red: the re-emitted rows get new seqs after the freeze and the stream file grows", "TestReArmReemissionOfUnchangedRowsAddsNoRecordAndKeepsTheFrozenWinners"},
	{"a change in attributes or body is written", "leave attrs or the event body out of sumOf", "red: the changed row returns the frozen winner's seq", "TestAChangedRowStillSupersedesTheFrozenWinnerWhileUnchangedRowsStay"},
	{"a higher revision is written even when unchanged", "drop the rev comparison in unchangedWinner", "red: a later lower revision wins over the dropped rev 3", "TestAHigherRevisionWithUnchangedContentIsRecordedSoALaterLowerRevisionStillLoses"},
	{"a dropped winner is not a no-op target", "drop the alive check in unchangedWinner", "red: the re-emission returns the dropped seq and reads nothing", "TestAnUnchangedReemissionOfADroppedWinnerIsRecordedAgain"},
	{"replay rebuilds the winner's content sum", "note an empty sum for replayed records", "red: after restart an unchanged row is written again", "TestRestartReplaysTheSameWinnersAndKeepsUnchangedReemissionsANoOp"},
}

func TestUnchangedMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "unchanged_test.go", unchangedMutations)
}

const servicesSource = "services"

func row(id, status string, rev int) contract.Signal {
	return withID(event(servicesSource, id, contract.Attrs{"table": "services", "status": status}), id, rev)
}

func streamBytes(t *testing.T, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, Dir, "stream.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func armed(t *testing.T, dir string) (*Stream, []int64, int64) {
	t.Helper()
	s := openAt(t, dir, DefaultCaps(), masker(t))
	var first []int64
	for _, id := range []string{"service-app", "service-db", "service-cache"} {
		first = append(first, mustAppend(t, s, row(id, "running", 1)))
	}
	mustAppend(t, s, metric("toy-db", 1))
	frozen, err := s.Freeze(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s, first, frozen.FrozenSeq
}

func TestReArmReemissionOfUnchangedRowsAddsNoRecordAndKeepsTheFrozenWinners(t *testing.T) {
	dir := runDir(t)
	s, first, frozenSeq := armed(t, dir)
	before := streamBytes(t, dir)
	var again []int64
	for _, id := range []string{"service-app", "service-db", "service-cache"} {
		again = append(again, mustAppend(t, s, row(id, "running", 1)))
	}
	if !slices.Equal(again, first) {
		t.Fatalf("re-emission returned %v, want the winners %v", again, first)
	}
	if after := streamBytes(t, dir); string(after) != string(before) {
		t.Fatalf("the stream file grew by %d bytes", len(after)-len(before))
	}
	got := readAll(t, s, 0)
	if !slices.Equal(seqs(got), []int64{first[0], first[1], first[2], first[2] + 1}) || got.Next != frozenSeq {
		t.Fatalf("read %v next %d, frozen at %d", seqs(got), got.Next, frozenSeq)
	}
	for _, sig := range got.Signals {
		if sig.Seq > frozenSeq {
			t.Fatalf("signal %d reads as after the bundle (frozen at %d)", sig.Seq, frozenSeq)
		}
	}
}

func TestAChangedRowStillSupersedesTheFrozenWinnerWhileUnchangedRowsStay(t *testing.T) {
	dir := runDir(t)
	s, first, frozenSeq := armed(t, dir)
	mustAppend(t, s, row("service-app", "running", 1))
	changed := mustAppend(t, s, row("service-db", "exited", 1))
	body := row("service-cache", "running", 1)
	body.Event.Body = "service-cache restarted"
	rebodied := mustAppend(t, s, body)
	if changed <= frozenSeq || rebodied != changed+1 {
		t.Fatalf("the changed rows got seqs %d and %d, not after the freeze at %d", changed, rebodied, frozenSeq)
	}
	got := readAll(t, s, 0)
	if want := []int64{first[0], first[2] + 1, changed, rebodied}; !slices.Equal(seqs(got), want) {
		t.Fatalf("read %v, want %v", seqs(got), want)
	}
	if seq, n := s.Revisions(servicesSource, contract.SignalKindEvent, "service-db"); seq != changed || n != 2 {
		t.Fatalf("service-db revisions %d %d", seq, n)
	}
	if seq, n := s.Revisions(servicesSource, contract.SignalKindEvent, "service-app"); seq != first[0] || n != 1 {
		t.Fatalf("service-app revisions %d %d", seq, n)
	}
}

func TestAHigherRevisionWithUnchangedContentIsRecordedSoALaterLowerRevisionStillLoses(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	mustAppend(t, s, row("service-app", "running", 1))
	raised := mustAppend(t, s, row("service-app", "running", 3))
	mustAppend(t, s, row("service-app", "exited", 2))
	got := readAll(t, s, 0)
	if len(got.Signals) != 1 || got.Signals[0].Seq != raised || got.Signals[0].Attrs["status"] != "running" {
		t.Fatalf("read %+v, want the rev 3 record %d", got.Signals, raised)
	}
}

func TestAnUnchangedReemissionOfADroppedWinnerIsRecordedAgain(t *testing.T) {
	s := openAt(t, runDir(t), Caps{Tail: DefaultTail, PerCollector: 2, PerContainer: DefaultPerContainer, CompactMin: 100}, masker(t))
	dropped := mustAppend(t, s, row("service-app", "running", 1))
	mustAppend(t, s, row("service-db", "running", 1))
	mustAppend(t, s, row("service-cache", "running", 1))
	if slices.Contains(seqs(readAll(t, s, 0)), dropped) {
		t.Fatalf("the quota kept %d, so the case does not hold", dropped)
	}
	back := mustAppend(t, s, row("service-app", "running", 1))
	if back == dropped || !slices.Contains(seqs(readAll(t, s, 0)), back) {
		t.Fatalf("the re-emission of a dropped winner returned %d (dropped %d) and reads %v", back, dropped, seqs(readAll(t, s, 0)))
	}
}

func TestRestartReplaysTheSameWinnersAndKeepsUnchangedReemissionsANoOp(t *testing.T) {
	dir := runDir(t)
	s, first, _ := armed(t, dir)
	mustAppend(t, s, row("service-app", "running", 1))
	changed := mustAppend(t, s, row("service-db", "exited", 1))
	live := readAll(t, s, 0)
	s.Close()
	again := openAt(t, dir, DefaultCaps(), masker(t))
	replayed := readAll(t, again, 0)
	if !slices.Equal(seqs(replayed), seqs(live)) || replayed.Next != live.Next {
		t.Fatalf("replayed %v next %d, live %v next %d", seqs(replayed), replayed.Next, seqs(live), live.Next)
	}
	before := streamBytes(t, dir)
	if seq := mustAppend(t, again, row("service-cache", "running", 1)); seq != first[2] {
		t.Fatalf("after restart an unchanged row returned %d, want %d", seq, first[2])
	}
	if seq := mustAppend(t, again, row("service-db", "exited", 1)); seq != changed {
		t.Fatalf("after restart the unchanged changed row returned %d, want %d", seq, changed)
	}
	if string(streamBytes(t, dir)) != string(before) {
		t.Fatal("after restart an unchanged re-emission wrote a record")
	}
}

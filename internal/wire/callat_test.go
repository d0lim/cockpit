package wire

import (
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var callAtMutations = []porttest.Mutation{
	{"a head is read only within the take limit", "drop the length and offset check in Store.CallAt", "red: the head with a five-gigabyte length is allocated and answers a read error", "TestCallAtRefusesAHeadOutsideTheTakeLimitBeforeItAllocates"},
}

func TestCallAtMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "callat_test.go", callAtMutations)
}

func TestCallAtRefusesAHeadOutsideTheTakeLimitBeforeItAllocates(t *testing.T) {
	s := OpenStore(t.TempDir())
	storedTake(t, s, SuiteTake)
	index, err := s.Index(SuiteTake)
	if err != nil || len(index.Heads) != 1 {
		t.Fatalf("index %+v, %v", index, err)
	}
	if _, err := s.CallAt(SuiteTake, index.Heads[0]); err != nil {
		t.Fatalf("the stored head does not read: %v", err)
	}
	refused := 0
	for name, head := range map[string]Head{
		"huge length":     {Seq: 1, Offset: 0, Length: 5 << 30},
		"negative length": {Seq: 1, Offset: 0, Length: -1},
		"negative offset": {Seq: 1, Offset: -1, Length: index.Heads[0].Length},
	} {
		_, err := s.CallAt(SuiteTake, head)
		if err == nil || !strings.Contains(err.Error(), "no readable line") {
			t.Errorf("%s: want the head refused before reading, got %v", name, err)
			continue
		}
		refused++
	}
	if refused != 3 {
		t.Fatalf("refused %d of 3", refused)
	}
}

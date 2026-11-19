package api

import (
	"errors"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/wire"
)

var wireCacheMutations = []porttest.Mutation{
	{"a take version is parsed once", "call load on every read", "red: the second read parses again", "TestTheCacheParsesATakeVersionOnceAndAgainOnlyWhenItChanges"},
	{"a changed index replaces the old one", "drop the version from the key in read", "red: the new version is not parsed", "TestTheCacheParsesATakeVersionOnceAndAgainOnlyWhenItChanges"},
	{"the cache holds at most four records", "drop the size loop from entry", "red: five records stay", "TestTheCacheKeepsFourRecordsAndDropsTheLeastRecentlyUsed"},
	{"an idle record leaves", "drop the idle comparison from entry", "red: the idle record stays", "TestARecordNobodyReadsForTenMinutesLeavesTheCache"},
	{"a failed parse is not kept", "keep the entry when load fails", "red: the failure is answered again", "TestAFailedParseIsNotKept"},
}

func TestWireCacheMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "wirecache_test.go", wireCacheMutations)
}

func wireKeyOf(run string, version int64) wireKey {
	return wireKey{run: run, take: wire.SuiteTake, version: wire.Version{Modified: version}}
}

func readCounted(c *wireCache, key wireKey, loads *int) {
	c.read(key, func() (*wireRecord, error) {
		*loads++
		return &wireRecord{}, nil
	})
}

func TestTheCacheParsesATakeVersionOnceAndAgainOnlyWhenItChanges(t *testing.T) {
	c := newWireCache(time.Now)
	loads := 0
	for range 3 {
		readCounted(c, wireKeyOf("a", 1), &loads)
	}
	if loads != 1 {
		t.Fatalf("%d parses of one version", loads)
	}
	readCounted(c, wireKeyOf("a", 2), &loads)
	if loads != 2 || len(c.entries) != 1 {
		t.Fatalf("%d parses, %d entries after a new version", loads, len(c.entries))
	}
}

func TestTheCacheKeepsFourRecordsAndDropsTheLeastRecentlyUsed(t *testing.T) {
	now := time.Unix(1000, 0)
	c := newWireCache(func() time.Time { return now })
	loads := 0
	tick := func(run string) {
		now = now.Add(time.Second)
		readCounted(c, wireKeyOf(run, 1), &loads)
	}
	for _, run := range []string{"a", "b", "c", "d"} {
		tick(run)
	}
	tick("a")
	tick("e")
	if len(c.entries) != wireCacheSize || loads != 5 {
		t.Fatalf("%d entries, %d parses", len(c.entries), loads)
	}
	if _, kept := c.entries[wireKeyOf("b", 1)]; kept {
		t.Fatal("the least recently used record stayed")
	}
	if _, kept := c.entries[wireKeyOf("a", 1)]; !kept {
		t.Fatal("canary: the record read again left")
	}
}

func TestARecordNobodyReadsForTenMinutesLeavesTheCache(t *testing.T) {
	now := time.Unix(1000, 0)
	c := newWireCache(func() time.Time { return now })
	loads := 0
	readCounted(c, wireKeyOf("a", 1), &loads)
	now = now.Add(wireCacheIdle - time.Second)
	readCounted(c, wireKeyOf("b", 1), &loads)
	if _, kept := c.entries[wireKeyOf("a", 1)]; !kept {
		t.Fatal("canary: a record inside the idle limit left")
	}
	now = now.Add(2 * time.Second)
	readCounted(c, wireKeyOf("b", 1), &loads)
	if _, kept := c.entries[wireKeyOf("a", 1)]; kept {
		t.Fatal("the idle record stayed")
	}
}

func TestAFailedParseIsNotKept(t *testing.T) {
	c := newWireCache(time.Now)
	failing := errors.New("broken")
	if _, err := c.read(wireKeyOf("a", 1), func() (*wireRecord, error) { return nil, failing }); !errors.Is(err, failing) {
		t.Fatalf("first read: %v", err)
	}
	rec, err := c.read(wireKeyOf("a", 1), func() (*wireRecord, error) { return &wireRecord{}, nil })
	if err != nil || rec == nil {
		t.Fatalf("the read after a failure: %v", err)
	}
}

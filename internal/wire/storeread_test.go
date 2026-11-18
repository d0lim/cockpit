package wire

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
)

func TestReadsKeepAnsweringWhileTheRecordMovesBetweenTheRunDirectoryAndTheBundle(t *testing.T) {
	const cycles = 150
	dir := t.TempDir()
	s := OpenStore(dir)
	storedTake(t, s, SuiteTake)
	storedTake(t, s, "exec-1")
	if err := os.Mkdir(filepath.Join(dir, BundleDir), dirMode); err != nil {
		t.Fatal(err)
	}
	var moving atomic.Bool
	moving.Store(true)
	var reads atomic.Int64
	var failure atomic.Value
	fail := func(what string, err any) { failure.CompareAndSwap(nil, what) }
	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			for moving.Load() {
				reads.Add(1)
				takes, err := s.Takes()
				if err != nil || !slices.Equal(takes, []string{"exec-1", SuiteTake}) {
					fail("Takes", err)
				}
				if _, err := s.IndexVersion(SuiteTake); err != nil {
					fail("IndexVersion", err)
				}
				index, err := s.Index(SuiteTake)
				if err != nil || len(index.Heads) != 1 {
					fail("Index", err)
					continue
				}
				if _, err := s.CallAt(SuiteTake, index.Heads[0]); err != nil {
					fail("CallAt", err)
				}
			}
		})
	}
	for range cycles {
		if err := s.Attach(); err != nil {
			t.Fatal(err)
		}
		if err := s.Detach(); err != nil {
			t.Fatal(err)
		}
	}
	moving.Store(false)
	readers.Wait()
	if what := failure.Load(); what != nil {
		t.Fatalf("%v failed while the record moved", what)
	}
	if reads.Load() == 0 {
		t.Fatal("no read ran while the record moved")
	}
}

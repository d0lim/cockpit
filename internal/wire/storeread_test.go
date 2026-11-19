package wire

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
)

func TestReadsKeepAnsweringWhileTheRecordMovesBetweenTheRunDirectoryAndTheBundle(t *testing.T) {
	const cycles, readerCount = 150, 4
	dir := t.TempDir()
	s := OpenStore(dir)
	storedTake(t, s, SuiteTake)
	storedTake(t, s, "exec-1")
	if err := os.Mkdir(filepath.Join(dir, BundleDir), dirMode); err != nil {
		t.Fatal(err)
	}
	var moving atomic.Bool
	moving.Store(true)
	var passes [readerCount]atomic.Int64
	var failure atomic.Value
	fail := func(what string) { failure.CompareAndSwap(nil, what) }
	var readers sync.WaitGroup
	for r := range readerCount {
		readers.Go(func() {
			for moving.Load() {
				takes, err := s.Takes()
				if err != nil || !slices.Equal(takes, []string{"exec-1", SuiteTake}) {
					fail("Takes")
				}
				if _, err := s.IndexVersion(SuiteTake); err != nil {
					fail("IndexVersion")
				}
				index, err := s.Index(SuiteTake)
				if err != nil || len(index.Heads) != 1 {
					fail("Index")
				} else if _, err := s.CallAt(SuiteTake, index.Heads[0]); err != nil {
					fail("CallAt")
				}
				passes[r].Add(1)
			}
		})
	}
	settle := func() {
		for r := range passes {
			for target := passes[r].Load() + 2; passes[r].Load() < target && failure.Load() == nil; {
				runtime.Gosched()
			}
		}
	}
	for range cycles {
		if err := s.Attach(); err != nil {
			t.Fatal(err)
		}
		settle()
		if err := s.Detach(); err != nil {
			t.Fatal(err)
		}
		settle()
	}
	moving.Store(false)
	readers.Wait()
	if what := failure.Load(); what != nil {
		t.Fatalf("%v failed while the record moved", what)
	}
	if passes[0].Load() < 2*cycles {
		t.Fatalf("%d read passes", passes[0].Load())
	}
}

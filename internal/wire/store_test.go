package wire

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var storeMutations = []porttest.Mutation{
	{"a report that is about to run moves the record out of the bundle", "skip moveTakes in Detach", "red: the record stays under report/ and is lost with it", "TestDetachAndAttachMoveTheRecordAndRootFollowsIt"},
	{"a detached record stays the root while a bundle exists", "prefer the bundle in Root", "red: takes land in the bundle during the report", "TestDetachAndAttachMoveTheRecordAndRootFollowsIt"},
	{"attach needs a bundle", "drop the bundle check in Attach", "red: a record is moved into a report directory that was discarded", "TestAttachWithoutABundleLeavesTheRecordInTheRunDirectory"},
	{"a take already in the target is not overwritten", "rename over an existing target in moveTakes", "red: rename fails on the kept take", "TestAMoveKeepsATakeAlreadyInTheTargetAndDropsAbandonedPartials"},
}

func TestStoreMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "store_test.go", storeMutations)
}

func storedTake(t *testing.T, s Store, name string) {
	t.Helper()
	stream := &fakeStream{lines: []string{callLine(t, plainCall(1, "a")), endLine(t, End{Calls: 1})}}
	in := takeInput(t, stream)
	in.Store, in.Name = s, name
	if status := Take(context.Background(), in); status.State != StateTaken {
		t.Fatalf("%s: %+v", name, status)
	}
}

func TestDetachAndAttachMoveTheRecordAndRootFollowsIt(t *testing.T) {
	dir := t.TempDir()
	s := OpenStore(dir)
	staging, bundle := filepath.Join(dir, StagingDir), filepath.Join(dir, BundleDir)
	if s.Root() != staging {
		t.Fatalf("root %s", s.Root())
	}
	storedTake(t, s, SuiteTake)
	if err := os.Mkdir(bundle, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(); err != nil {
		t.Fatal(err)
	}
	attached := filepath.Join(bundle, StagingDir)
	if s.Root() != attached || !isFile(filepath.Join(attached, SuiteTake, IndexFile)) || exists(staging) {
		t.Fatalf("after attach: root %s", s.Root())
	}
	storedTake(t, s, "exec-1")
	if !isFile(filepath.Join(attached, "exec-1", IndexFile)) {
		t.Fatal("a later take did not land in the bundle")
	}
	if err := s.Detach(); err != nil {
		t.Fatal(err)
	}
	if s.Root() != staging || exists(attached) {
		t.Fatalf("after detach: root %s", s.Root())
	}
	takes, err := s.Takes()
	if err != nil || !slices.Equal(takes, []string{"exec-1", SuiteTake}) {
		t.Fatalf("takes %v: %v", takes, err)
	}
	storedTake(t, s, "during-report")
	if !isFile(filepath.Join(staging, "during-report", IndexFile)) {
		t.Fatal("a take during the report did not stay out of the bundle")
	}
}

func TestAttachWithoutABundleLeavesTheRecordInTheRunDirectory(t *testing.T) {
	dir := t.TempDir()
	s := OpenStore(dir)
	storedTake(t, s, SuiteTake)
	if err := s.Attach(); err != nil {
		t.Fatal(err)
	}
	if !isFile(filepath.Join(dir, StagingDir, SuiteTake, IndexFile)) || exists(filepath.Join(dir, BundleDir)) {
		t.Fatal("the record moved without a bundle")
	}
	if err := OpenStore(t.TempDir()).Attach(); err != nil {
		t.Fatalf("a run without a record: %v", err)
	}
	if err := OpenStore(t.TempDir()).Detach(); err != nil {
		t.Fatalf("a run without a bundle: %v", err)
	}
}

func TestAMoveKeepsATakeAlreadyInTheTargetAndDropsAbandonedPartials(t *testing.T) {
	dir := t.TempDir()
	s := OpenStore(dir)
	storedTake(t, s, SuiteTake)
	bundle := filepath.Join(dir, BundleDir, StagingDir)
	if err := os.MkdirAll(filepath.Join(bundle, SuiteTake), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, SuiteTake, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, StagingDir, partialPrefix+"x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(); err != nil {
		t.Fatal(err)
	}
	if !isFile(filepath.Join(bundle, SuiteTake, "keep")) || isFile(filepath.Join(bundle, SuiteTake, IndexFile)) || exists(filepath.Join(dir, StagingDir)) || exists(filepath.Join(bundle, partialPrefix+"x")) {
		t.Fatal("the move replaced a kept take or kept a partial")
	}
}

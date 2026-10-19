package signal

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var kvMutations = []porttest.Mutation{
	{"collector ids that are not path-safe names are refused", "skip ValidCollector in kvName", "red: ../escape writes outside kv/", "TestCollectorStateRefusesIDsThatAreNotPathSafeNames"},
	{"collector state has a size limit", "drop the KVMaxLen check", "red: a 2 MiB state is written", "TestCollectorStateOverTheSizeLimitIsRefused"},
	{"collector state survives a restart with an atomic write", "write in place without the temporary file", "red: a crash between truncate and write loses the state", "TestCollectorStatePersistsAcrossRestartUnderKV"},
}

func TestCollectorStatePersistsAcrossRestartUnderKV(t *testing.T) {
	dir := runDir(t)
	m := masker(t)
	s := openAt(t, dir, DefaultCaps(), m)
	if got, err := s.KVGet("toy-db"); err != nil || len(got) != 0 {
		t.Fatalf("%v %v", got, err)
	}
	want := map[string]any{"cursor": 42.0, "seen": []any{"a", "b"}}
	if err := s.KVPut("toy-db", want); err != nil {
		t.Fatal(err)
	}
	s.Close()
	again := openAt(t, dir, DefaultCaps(), m)
	got, err := again.KVGet("toy-db")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, Dir, "kv", "toy-db.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, Dir, "kv", "toy-db.json.tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("temporary file left behind")
	}
}

func TestCollectorStateRefusesIDsThatAreNotPathSafeNames(t *testing.T) {
	dir := runDir(t)
	s := openAt(t, dir, DefaultCaps(), masker(t))
	for _, id := range []string{"../escape", "a/b", "Toy", "", "-lead", strings.Repeat("a", 64)} {
		if err := s.KVPut(id, map[string]any{"x": 1.0}); err == nil {
			t.Errorf("put %q accepted", id)
		}
		if _, err := s.KVGet(id); err == nil {
			t.Errorf("get %q accepted", id)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "escape.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a file escaped kv/")
	}
	if err := s.KVPut(strings.Repeat("a", 63), map[string]any{}); err != nil {
		t.Fatalf("canary: a 63-character id was refused: %v", err)
	}
}

func TestCollectorStateOverTheSizeLimitIsRefused(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	if err := s.KVPut("toy-db", map[string]any{"blob": strings.Repeat("x", 2<<20)}); err == nil {
		t.Fatal("2 MiB state accepted")
	}
	if err := s.KVPut("toy-db", map[string]any{"blob": strings.Repeat("x", 1000)}); err != nil {
		t.Fatalf("canary: %v", err)
	}
}

func TestKVMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "kv_test.go", kvMutations)
}

package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

func TestApplyHubUmaskSets077AndReturnsThePreviousValue(t *testing.T) {
	previous := ApplyHubUmask()
	during := syscall.Umask(previous)
	if during != 0o077 {
		t.Fatalf("umask %o", during)
	}
}

func atomicTarget(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func tmpOf(path string) string { return path + "." + strconv.Itoa(os.Getpid()) + ".tmp" }

func TestWriteAtomicReplacesTheFileWithMode600AndLeavesNoTemporaryFile(t *testing.T) {
	path := atomicTarget(t)
	if err := WriteAtomic(path, []byte("new content")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new content" || mode(t, path) != 0o600 {
		t.Fatalf("%q %o", got, mode(t, path))
	}
	if _, err := os.Stat(tmpOf(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file left: %v", err)
	}
}

func TestWriteAtomicFailureMidWriteKeepsTheOriginalAndLeavesOnlyTheTemporaryFile(t *testing.T) {
	path := atomicTarget(t)
	halfThenFail := func(f *os.File, data []byte) error {
		f.Write(data[:3])
		return errors.New("disk full")
	}
	if err := writeAtomic(path, []byte("new content"), halfThenFail); err == nil || err.Error() != "disk full" {
		t.Fatalf("err %v", err)
	}
	original, _ := os.ReadFile(path)
	partial, _ := os.ReadFile(tmpOf(path))
	if string(original) != "old" || string(partial) != "new" || mode(t, tmpOf(path)) != 0o600 {
		t.Fatalf("%q %q", original, partial)
	}
	if err := WriteAtomic(path, []byte("new content")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tmpOf(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("next write left the temporary file")
	}
}

func TestRunDirIsUnderRunsForARunIDAndRefusesAnythingElse(t *testing.T) {
	s := Open(homeFixture(t))
	dir, err := s.RunDir(runA)
	if err != nil || dir != filepath.Join(s.Home(), "runs", runA) {
		t.Fatalf("%q %v", dir, err)
	}
	if _, err := s.RunDir("../x"); err == nil || !strings.Contains(err.Error(), "not a run id") {
		t.Fatalf("err %v", err)
	}
}

func TestMetaRoundTripsWithDirectories700AndFile600(t *testing.T) {
	s := Open(homeFixture(t))
	want := fixture(runA, func(m *Meta) {
		m.State = lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictPassed}
		m.Slot = slot(2)
		m.Keep = true
		m.RunnerImage = &contract.RunnerImage{Tag: "cockpit-runner:4d9bba6a1623", ID: "sha256:1"}
	})
	mustWriteMeta(t, s, want)
	got, err := s.ReadMeta(runA)
	if err != nil || !reflect.DeepEqual(*got, want) {
		t.Fatalf("%+v %v", got, err)
	}
	dir, _ := s.RunDir(runA)
	for _, d := range []string{s.Home(), filepath.Join(s.Home(), "runs"), dir} {
		if mode(t, d) != 0o700 {
			t.Fatalf("%s mode %o", d, mode(t, d))
		}
	}
	if mode(t, filepath.Join(dir, "meta.json")) != 0o600 {
		t.Fatal("meta mode")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "meta.json"))
	for _, key := range []string{`"cockpitVersion": "v0.3.0"`, `"hostApi": 1`, `"tag": "cockpit-runner:4d9bba6a1623"`, `"id": "sha256:1"`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("meta lacks %s", key)
		}
	}
	if strings.Contains(string(raw), `"harness"`) {
		t.Fatal("meta carries the previous layout's harness field")
	}
}

func TestReadMetaIsNilForAMissingRunAndAnErrorNamingTheRunForBrokenJSON(t *testing.T) {
	s := Open(homeFixture(t))
	if m, err := s.ReadMeta(runA); m != nil || err != nil {
		t.Fatalf("%v %v", m, err)
	}
	dir, _ := s.RunDir(runA)
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "meta.json"), []byte("{"), 0o600)
	if _, err := s.ReadMeta(runA); err == nil || !strings.Contains(err.Error(), runA) {
		t.Fatalf("err %v", err)
	}
}

func TestStateRoundTripsWithFile600AndAMissingFileIsTheEmptyState(t *testing.T) {
	s := Open(homeFixture(t))
	if got := s.ReadState(); !reflect.DeepEqual(got, StateRead{State: EmptyState()}) {
		t.Fatalf("%+v", got)
	}
	want := HubState{SchemaVersion: 2, Queue: []string{runB}, Leases: Leases{runA: 3}}
	if err := s.WriteState(want); err != nil {
		t.Fatal(err)
	}
	if got := s.ReadState(); !reflect.DeepEqual(got, StateRead{State: want}) {
		t.Fatalf("%+v", got)
	}
	if mode(t, filepath.Join(s.Home(), "state.json")) != 0o600 || mode(t, s.Home()) != 0o700 {
		t.Fatal("modes")
	}
}

func TestListRunsRebuildsFromMetaAndReportsABrokenStateFile(t *testing.T) {
	cases := []struct{ name, text, reason string }{
		{"not JSON", `{"queue": [`, "is not JSON"},
		{"other schema version", `{"schemaVersion":1,"queue":[],"leases":{}}`, "schema version 2"},
		{"queue entry that is not a run id", `{"schemaVersion":2,"queue":["../x"],"leases":{}}`, "schema version 2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Open(homeFixture(t))
			mustWriteMeta(t, s, fixture(runB, func(m *Meta) { m.CreatedAt = "2026-10-12T06:30:00.400Z" }))
			mustWriteMeta(t, s, fixture(runA, func(m *Meta) {
				m.CreatedAt = "2026-10-12T06:30:00.100Z"
				m.State = lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvStopped, Verdict: lifecycle.VerdictFailed}
				m.Slot = slot(1)
			}))
			os.WriteFile(filepath.Join(s.Home(), "state.json"), []byte(c.text), 0o600)
			list, err := s.ListRuns()
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{list.Runs[0].ID, list.Runs[1].ID}
			if !slices.Equal(ids, []string{runA, runB}) || *list.Runs[0].Slot != 1 || !strings.Contains(list.StateBroken, c.reason) {
				t.Fatalf("%v %q", ids, list.StateBroken)
			}
			if !reflect.DeepEqual(list.State, EmptyState()) {
				t.Fatalf("%+v", list.State)
			}
		})
	}
}

func TestListRunsSkipsEntriesThatAreNotRunIDsAndDirectoriesWithoutMeta(t *testing.T) {
	s := Open(homeFixture(t))
	mustWriteMeta(t, s, fixture(runA))
	os.MkdirAll(filepath.Join(s.Home(), "runs", "notes"), 0o700)
	os.MkdirAll(filepath.Join(s.Home(), "runs", runB), 0o700)
	os.WriteFile(filepath.Join(s.Home(), "runs", ".DS_Store"), nil, 0o600)
	list, err := s.ListRuns()
	if err != nil || len(list.Runs) != 1 || list.Runs[0].ID != runA || list.StateBroken != "" {
		t.Fatalf("%+v %v", list, err)
	}
}

func TestListRunsOfAnEmptyHomeIsEmpty(t *testing.T) {
	list, err := Open(homeFixture(t)).ListRuns()
	if err != nil || len(list.Runs) != 0 {
		t.Fatalf("%+v %v", list, err)
	}
}

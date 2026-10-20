package lock

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var lockMutations = []porttest.Mutation{
	{"a live owner refuses the second hub", "treat every pid as dead in alive", "red: the second acquire takes over a live hub's lock", "TestLiveOwnerRefusesAcquireWithItsAddressAndKeepsTheLock"},
	{"a dead owner's lock is taken over", "treat every pid as alive", "red: a dead hub's lock blocks the start", "TestDeadOwnersLockIsTakenOverWithThisPidAndPort"},
	{"an unreadable lock is never taken over", "default an unparsable lock to pid 0 and take it", "red: an empty lock file is replaced", "TestUnreadableLockIsNotTakenOverAndNamesThePathToDelete"},
	{"release removes only this process's lock", "remove the file without the pid check", "red: another pid's lock is deleted", "TestReleaseRemovesOnlyThisProcessLock"},
	{"the lock is created exclusively with mode 600", "open without O_EXCL", "red: the second create overwrites", "TestAcquireOnAnEmptyHomeCreatesALockWithPidPortAndModes"},
	{"re-acquire in the same process is re-entrant", "skip the registry", "red: the second acquire rewrites the file", "TestSecondAcquireInTheSameProcessReturnsTheSameLock"},
	{"a lock file naming this pid is re-entrant", "drop the own-pid branch", "red: the hub refuses its own lock", "TestLockFileWithThisPidIsReentrant"},
	{"a holder is reported only while its process lives", "drop the alive check in Holder", "red: a dead pid's lock reads as a running hub", "TestHolderNamesALiveOwnersPortAndNothingForADeadOrMissingLock"},
	{"the hub listens on loopback only", "use COCKPIT_PORT as the whole address", "red: 0.0.0.0:80 is accepted", "TestListenAddressIsLoopbackOnlyAndRejectsHostsAndBadPorts"},
}

const started = "2026-10-12T06:00:00.000Z"

func homeFixture(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	t.Cleanup(func() { Release(home) })
	return home
}

func plant(t *testing.T, home, text string) {
	t.Helper()
	os.MkdirAll(home, 0o700)
	if err := os.WriteFile(filepath.Join(home, "hub.lock"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func body(pid, port int) string {
	data, _ := json.Marshal(map[string]any{"pid": pid, "port": port, "startedAt": started})
	return string(data)
}

func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

func read(t *testing.T, home string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, "hub.lock"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAcquireOnAnEmptyHomeCreatesALockWithPidPortAndModes(t *testing.T) {
	home := homeFixture(t)
	l, err := Acquire(home, 7710)
	if err != nil || l.PID != os.Getpid() || l.Port != 7710 {
		t.Fatalf("%+v %v", l, err)
	}
	var onDisk Lock
	json.Unmarshal([]byte(read(t, home)), &onDisk)
	if onDisk.PID != os.Getpid() || onDisk.Port != 7710 || onDisk.StartedAt != l.StartedAt {
		t.Fatalf("%+v", onDisk)
	}
	lockInfo, _ := os.Stat(filepath.Join(home, "hub.lock"))
	homeInfo, _ := os.Stat(home)
	if lockInfo.Mode().Perm() != 0o600 || homeInfo.Mode().Perm() != 0o700 {
		t.Fatalf("%o %o", lockInfo.Mode().Perm(), homeInfo.Mode().Perm())
	}
}

func TestSecondAcquireInTheSameProcessReturnsTheSameLock(t *testing.T) {
	home := homeFixture(t)
	first, _ := Acquire(home, 7710)
	before := read(t, home)
	second, err := Acquire(home, 7710)
	if err != nil || second != first || read(t, home) != before {
		t.Fatalf("%v %v", second, err)
	}
}

func TestLockFileWithThisPidIsReentrant(t *testing.T) {
	home := homeFixture(t)
	plant(t, home, body(os.Getpid(), 7711))
	l, err := Acquire(home, 7710)
	if err != nil || l.PID != os.Getpid() || l.Port != 7711 || l.StartedAt != started {
		t.Fatalf("%+v %v", l, err)
	}
}

func TestLiveOwnerRefusesAcquireWithItsAddressAndKeepsTheLock(t *testing.T) {
	home := homeFixture(t)
	other := exec.Command("sleep", "30")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Process.Kill(); other.Wait() })
	plant(t, home, body(other.Process.Pid, 7799))
	before := read(t, home)
	_, err := Acquire(home, 7710)
	if err == nil || !strings.Contains(err.Error(), "a hub is already running: http://127.0.0.1:7799 (pid "+strconv.Itoa(other.Process.Pid)) {
		t.Fatalf("err %v", err)
	}
	if read(t, home) != before {
		t.Fatal("lock changed")
	}
}

func TestDeadOwnersLockIsTakenOverWithThisPidAndPort(t *testing.T) {
	home := homeFixture(t)
	plant(t, home, body(deadPID(t), 7799))
	l, err := Acquire(home, 7710)
	if err != nil || l.PID != os.Getpid() || l.Port != 7710 {
		t.Fatalf("%+v %v", l, err)
	}
	var onDisk Lock
	json.Unmarshal([]byte(read(t, home)), &onDisk)
	if onDisk.PID != os.Getpid() || onDisk.Port != 7710 {
		t.Fatalf("%+v", onDisk)
	}
}

func TestUnreadableLockIsNotTakenOverAndNamesThePathToDelete(t *testing.T) {
	cases := map[string]string{"empty file": "", "pid zero": body(0, 7799), "missing port": `{"pid":12,"startedAt":"x"}`}
	for name, text := range cases {
		home := homeFixture(t)
		plant(t, home, text)
		_, err := Acquire(home, 7710)
		abs, _ := filepath.Abs(home)
		want := "cannot read hub.lock: if no hub is running, delete " + filepath.Join(abs, "hub.lock") + " and start again"
		if err == nil || err.Error() != want {
			t.Fatalf("%s: err %v, want %q", name, err, want)
		}
		if read(t, home) != text {
			t.Fatalf("%s: lock changed", name)
		}
	}
	if len(cases) != 3 {
		t.Fatal("case count")
	}
}

func TestReleaseRemovesOnlyThisProcessLock(t *testing.T) {
	home := homeFixture(t)
	first, _ := Acquire(home, 7710)
	Release(home)
	if _, err := os.Stat(filepath.Join(home, "hub.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("own lock left behind")
	}
	second, err := Acquire(home, 7711)
	if err != nil || second == first || second.Port != 7711 {
		t.Fatalf("%+v %v", second, err)
	}
	Release(home)
	other := homeFixture(t)
	text := body(deadPID(t), 7799)
	plant(t, other, text)
	Release(other)
	if read(t, other) != text {
		t.Fatal("another pid's lock was removed")
	}
}

func TestHolderNamesALiveOwnersPortAndNothingForADeadOrMissingLock(t *testing.T) {
	missing := homeFixture(t)
	if l, ok := Holder(missing); ok || l != nil {
		t.Fatalf("missing lock: %+v %v", l, ok)
	}
	live := homeFixture(t)
	other := exec.Command("sleep", "30")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Process.Kill(); other.Wait() })
	plant(t, live, body(other.Process.Pid, 7801))
	if l, ok := Holder(live); !ok || l.Port != 7801 || l.PID != other.Process.Pid {
		t.Fatalf("live lock: %+v %v", l, ok)
	}
	dead := homeFixture(t)
	plant(t, dead, body(deadPID(t), 7802))
	if l, ok := Holder(dead); ok || l != nil {
		t.Fatalf("dead lock: %+v %v", l, ok)
	}
}

func TestListenAddressIsLoopbackOnlyAndRejectsHostsAndBadPorts(t *testing.T) {
	env := func(v string) func(string) string { return func(string) string { return v } }
	allow := map[string]string{"": "127.0.0.1:7710", "7711": "127.0.0.1:7711", "65535": "127.0.0.1:65535"}
	for v, want := range allow {
		if got, _, err := ListenAddr(env(v)); err != nil || got != want {
			t.Errorf("%q: %q %v", v, got, err)
		}
	}
	deny := []string{"0.0.0.0:80", ":7710", "localhost:7710", "0", "-1", "70000", "abc", " 7711", "07711"}
	for _, v := range deny {
		if got, _, err := ListenAddr(env(v)); err == nil {
			t.Errorf("%q accepted as %q", v, got)
		}
	}
	if len(allow)+len(deny) != 12 {
		t.Fatal("case count")
	}
}

func TestLockMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "lock_test.go", lockMutations)
}

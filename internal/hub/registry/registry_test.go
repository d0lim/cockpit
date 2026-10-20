package registry

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lock"
	"github.com/d0lim/cockpit/internal/hub/store"
)

const runA = "run-20261020-101500-abc1234"

func homes(t *testing.T, names ...string) (string, []string) {
	t.Helper()
	root := t.TempDir()
	var out []string
	for _, n := range names {
		home := filepath.Join(root, n)
		if err := os.MkdirAll(home, 0o700); err != nil {
			t.Fatal(err)
		}
		out = append(out, home)
	}
	return filepath.Join(root, "registry"), out
}

func plantLock(t *testing.T, home string, pid, port int) {
	t.Helper()
	text := `{"pid":` + strconv.Itoa(pid) + `,"port":` + strconv.Itoa(port) + `,"startedAt":"2026-10-20T01:00:00.000Z"}`
	if err := os.WriteFile(filepath.Join(home, "hub.lock"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func liveProcess(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	return cmd.Process.Pid
}

func TestRegistryDirComesFromCockpitRegistryThenXdgStateHomeAndIgnoresCockpitHome(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got, err := Dir(env(map[string]string{EnvRegistry: "/r", xdgState: "/y", "COCKPIT_HOME": "/h"})); err != nil || got != "/r" {
		t.Fatalf("%q %v", got, err)
	}
	if got, err := Dir(env(map[string]string{xdgState: "/y", "COCKPIT_HOME": "/h"})); err != nil || got != filepath.Join("/y", "cockpit-hubs") {
		t.Fatalf("%q %v", got, err)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	if got, err := Dir(env(map[string]string{"COCKPIT_HOME": "/h"})); err != nil || got != filepath.Join(userHome, ".local", "state", "cockpit-hubs") {
		t.Fatalf("%q %v", got, err)
	}
}

func TestTwoRegisteredHubsListThisHubFirstWithLiveStateFromTheirLocks(t *testing.T) {
	dir, hs := homes(t, "zeta", "alpha", "beta")
	self, up, down := hs[0], hs[1], hs[2]
	for _, h := range hs {
		if err := Open(dir, h).Register(); err != nil {
			t.Fatal(err)
		}
	}
	plantLock(t, up, liveProcess(t), 7811)
	got := Open(dir, self).Hubs()
	if len(got) != 3 {
		t.Fatalf("%d hubs: %+v", len(got), got)
	}
	want := []Hub{
		{ID: ID(self), Name: "zeta", Home: self, Self: true},
		{ID: ID(up), Name: "alpha", Home: up, Up: true, Port: 7811},
		{ID: ID(down), Name: "beta", Home: down},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("hub %d: got %+v want %+v", i, got[i], want[i])
		}
	}
	if got[1].URL() != "http://127.0.0.1:7811" || got[2].URL() != "" {
		t.Fatalf("urls %q %q", got[1].URL(), got[2].URL())
	}
}

func TestAnEntryWhoseHomeIsGoneOrWhoseIdDoesNotMatchItsHomeIsSkipped(t *testing.T) {
	dir, hs := homes(t, "self", "gone", "kept")
	for _, h := range hs {
		if err := Open(dir, h).Register(); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(hs[1]); err != nil {
		t.Fatal(err)
	}
	forged := filepath.Join(dir, "000000000000.json")
	if err := os.WriteFile(forged, []byte(`{"home":"`+hs[2]+`","name":"forged"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := Open(dir, hs[0]).Hubs()
	if len(got) != 2 || got[0].Home != hs[0] || got[1].Home != hs[2] || got[1].Name != "kept" {
		t.Fatalf("%+v", got)
	}
}

func TestRegisteringAgainRewritesTheSameEntry(t *testing.T) {
	dir, hs := homes(t, "self")
	r := Open(dir, hs[0])
	for range 2 {
		if err := r.Register(); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != ID(hs[0])+".json" {
		t.Fatalf("%v %v", entries, err)
	}
}

func TestAnOrphanBelongsToTheOtherHubThatHoldsItsRecordAndToNoHubOtherwise(t *testing.T) {
	dir, hs := homes(t, "self", "other")
	for _, h := range hs {
		if err := Open(dir, h).Register(); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Open(hs[1]).WriteMeta(store.Meta{ID: runA}); err != nil {
		t.Fatal(err)
	}
	if err := store.Open(hs[0]).WriteMeta(store.Meta{ID: "run-20261020-101600-def5678"}); err != nil {
		t.Fatal(err)
	}
	hubs := Open(dir, hs[0]).Hubs()
	if got := Owner(hubs, runA); got != ID(hs[1]) {
		t.Fatalf("owner %q", got)
	}
	if got := Owner(hubs, "run-20261020-101600-def5678"); got != "" {
		t.Fatalf("own run owned by %q", got)
	}
	if got := Owner(hubs, "run-20261020-101700-0000000"); got != "" {
		t.Fatalf("unknown run owned by %q", got)
	}
	if got := Owner(hubs, "../other"); got != "" {
		t.Fatalf("non run id owned by %q", got)
	}
}

func TestHubIDIsStableForOneHomeAndDiffersBetweenHomes(t *testing.T) {
	_, hs := homes(t, "a", "b")
	if ID(hs[0]) != ID(hs[0]+"/.") || ID(hs[0]) == ID(hs[1]) || len(ID(hs[0])) != 12 {
		t.Fatalf("%q %q", ID(hs[0]), ID(hs[1]))
	}
	if _, ok := lock.Holder(hs[0]); ok {
		t.Fatal("a home without a lock reads as up")
	}
}
